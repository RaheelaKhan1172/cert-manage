// +build darwin

package store

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/adamdecaf/cert-manage/tools/_x509"
	"github.com/adamdecaf/cert-manage/tools/file"
	"github.com/adamdecaf/cert-manage/tools/pem"
	"github.com/adamdecaf/cert-manage/whitelist"
)

var (
	plistModDateFormat = "2006-01-02T15:04:05Z"
	systemDirs         = []string{
		"/System/Library/Keychains/SystemRootCertificates.keychain",
		"/Library/Keychains/System.keychain",
	}

	// internal options
	debug = strings.Contains(os.Getenv("GODEBUG"), "x509roots=1")
)

const (
	backupDirPerms = 0744
	plistFilePerms = 0644
)

// darwinStore represents the structure of a `store.Store`, but for the darwin (OSX and
// macOS) platform.
//
// Within the code a cli tool called `security` is often used to extract and modify the
// trust settings of installed certificates in the various Keychains.
//
// https://developer.apple.com/legacy/library/documentation/Darwin/Reference/ManPages/man1/security.1.html
type darwinStore struct{}

func platform() Store {
	return darwinStore{}
}

// Backup will save off a copy of the existing trust policy
func (s darwinStore) Backup() error {
	fd, err := trustSettingsExport()
	if fd != nil {
		defer os.Remove(fd.Name())
	}
	if err != nil {
		return err
	}

	// Copy the temp file somewhere safer
	outDir, err := getCertManageDir()
	if err != nil {
		return err
	}
	filename := fmt.Sprintf("trust-backup-%d.xml", time.Now().Unix())
	out := filepath.Join(outDir, filename)

	// Copy file
	err = os.MkdirAll(outDir, backupDirPerms)
	if err != nil {
		return err
	}
	err = file.CopyFile(fd.Name(), out)
	return err
}

// List
//
// Note: Currently we are ignoring the login keychain. This is done because those certs are
// typically modified by the user (or an application the user trusts).
func (s darwinStore) List() ([]*x509.Certificate, error) {
	installed, err := readInstalledCerts(systemDirs...)
	if err != nil {
		return nil, err
	}
	trustItems, err := getCertsWithTrustPolicy()
	if err != nil {
		return nil, err
	}

	if debug {
		fmt.Printf("%d installed, %d with policy\n", len(installed), len(trustItems))
	}

	kept := make([]*x509.Certificate, 0)
	for i := range installed {
		if installed[i] == nil {
			continue
		}
		if trustItems.contains(installed[i]) {
			kept = append(kept, installed[i])
			continue
		}
	}

	return kept, nil
}

// readInstalledCerts pulls certificates from the `security` cli tool that's
// installed. This will return certificates, but not their trust status.
func readInstalledCerts(paths ...string) ([]*x509.Certificate, error) {
	res := make([]*x509.Certificate, 0)

	args := []string{"find-certificate", "-a", "-p"}
	args = append(args, paths...)

	b, err := exec.Command("/usr/bin/security", args...).Output()
	if err != nil {
		return nil, err
	}

	cs, err := pem.Parse(b)
	if err != nil {
		return nil, err
	}
	for _, c := range cs {
		if c == nil {
			continue
		}
		add := true
		for i := range res {
			if reflect.DeepEqual(c.Signature, res[i].Signature) {
				add = false
				break
			}
		}
		if add {
			res = append(res, c)
		}
	}

	return res, nil
}

func getCertsWithTrustPolicy() (trustItems, error) {
	fd, err := trustSettingsExport()
	defer os.Remove(fd.Name())
	if err != nil {
		return nil, err
	}

	plist, err := parsePlist(fd)
	if err != nil {
		return nil, err
	}

	return plist.convertToTrustItems(), nil
}

// trustSettingsExport calls out to the `security` cli tool and
// returns an os.File for the plist file written
//
// Note: Callers are expected to cleanup the file handler
func trustSettingsExport() (*os.File, error) {
	// Create temp file for plist output
	fd, err := ioutil.TempFile("", "trust-settings")
	if err != nil {
		return nil, err
	}

	// build up command arguments
	args := append([]string{
		"trust-settings-export",
		"-d", fd.Name(),
	})

	// run command
	_, err = exec.Command("/usr/bin/security", args...).Output()
	if err != nil {
		return nil, err
	}

	return fd, nil
}

func (s darwinStore) Remove(wh whitelist.Whitelist) error {
	certs, err := s.List()
	if err != nil {
		return err
	}

	// Keep what's whitelisted
	kept := make([]*x509.Certificate, 0)
	for i := range certs {
		if wh.Matches(certs[i]) {
			kept = append(kept, certs[i])
		}
	}

	// Build plist xml file and restore on the system
	trustItems := make(trustItems, 0)
	for i := range kept {
		if kept[i] == nil {
			continue
		}
		trustItems = append(trustItems, trustItemFromCertificate(*kept[i]))
	}

	// Create temporary output file
	f, err := ioutil.TempFile("", "cert-manage")
	defer os.Remove(f.Name())
	if err != nil {
		return err
	}

	// Write out plist file
	// TODO(adam): This needs to have set the trust settings (to Never Trust), the <array> fields lower on
	err = trustItems.toXmlFile(f.Name())
	if err != nil {
		return err
	}

	return s.Restore(f.Name())
}

// TODO(adam): This should default trust to "Use System Trust", not "Always Trust"
// Maybe this is a change for "Backup"...?
func (s darwinStore) Restore(where string) error {
	// Setup file to use as restore point
	if where == "" {
		// Ignore any errors and try to set a file
		latest, _ := getLatestBackupFile()
		where = latest
	}
	if where == "" {
		// No backup dir (or backup files) and no -file specified
		return errors.New("No backup file found and -file not specified")
	}
	if !file.Exists(where) {
		return errors.New("Restore file doesn't exist")
	}

	// run restore
	args := []string{"/usr/bin/security", "trust-settings-import", "-d", where}
	_, err := exec.Command("sudo", args...).Output()

	return err
}

func getUserKeychainPaths() ([]string, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}

	return []string{
		filepath.Join(u.HomeDir, "/Library/Keychains/login.keychain"),
		filepath.Join(u.HomeDir, "/Library/Keychains/login.keychain-db"),
	}, nil
}

func getCertManageDir() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return filepath.Join(u.HomeDir, "/Library/cert-manage"), nil
}

func getLatestBackupFile() (string, error) {
	dir, err := getCertManageDir()
	if err != nil {
		return "", err
	}
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		return "", err
	}
	if len(fis) == 0 {
		return "", nil
	}

	// get largest
	file.SortFileInfos(fis)
	latest := fis[len(fis)-1]
	return filepath.Join(dir, latest.Name()), nil
}

// trustItems wraps up a collection of trustItems parsed from the `security` cli tool
type trustItems []trustItem

func (t trustItems) contains(cert *x509.Certificate) bool {
	if cert == nil {
		// we don't want to say we've got a nil cert
		return true
	}
	fp := _x509.GetHexSHA1Fingerprint(*cert)
	for i := range t {
		if fp == t[i].sha1Fingerprint {
			return true
		}
	}
	return false
}

func (t trustItems) toXmlFile(where string) error {
	// Due to a known limitation of encoding/xml it often doesn't
	// follow the ordering of slices. To work around this we've decided
	// to build the xml in a more manual fashion.
	// https://golang.org/pkg/encoding/xml/#pkg-note-BUG

	// Write the header
	header := []byte(`<plist><dict><key>trustList</key><dict>`)
	itemEnd := []byte("</dict>")
	footer := []byte(`</dict>
  <key>trustVersion</key>
  <integer>1</integer>
</dict></plist>`)

	// Build up the inner contents
	out := make([]byte, 0)
	for i := 0; i < len(t); i += 1 {
		key := []byte(fmt.Sprintf("<key>%s</key>", strings.ToUpper(t[i].sha1Fingerprint)))

		// issuerName
		rdn := t[i].issuerName.ToRDNSequence()
		bs, _ := asn1.Marshal(rdn)
		issuer := []byte(fmt.Sprintf("<key>issuerName</key><data>%s</data>", base64.StdEncoding.EncodeToString(bs)))

		// modDate
		modDate := []byte(fmt.Sprintf("<key>modDate</key><date>%s</date>", t[i].modDate.Format(plistModDateFormat)))

		// serialNumber
		serial := []byte(fmt.Sprintf("<key>serialNumber</key><data>%s</data>", base64.StdEncoding.EncodeToString(t[i].serialNumber)))

		// Build item
		inner := append(key, []byte("<dict>")...)
		inner = append(inner, issuer...)
		inner = append(inner, modDate...)
		inner = append(inner, serial...)
		inner = append(inner, itemEnd...)

		// Ugh, join them all together
		out = append(out, inner...)
	}

	// write xml file out
	content := append(header, append(out, footer...)...)
	return ioutil.WriteFile(where, content, plistFilePerms)
}

// trustItem represents an entry from the plist (xml) files produced by
// the /usr/bin/security cli tool
type trustItem struct {
	// required
	sha1Fingerprint string
	issuerName      pkix.Name
	modDate         time.Time
	serialNumber    []byte

	// optional
	// TODO(adam): needs picked up?
	kSecTrustSettingsResult int32
}

func trustItemFromCertificate(cert x509.Certificate) trustItem {
	return trustItem{
		sha1Fingerprint: _x509.GetHexSHA1Fingerprint(cert),
		issuerName:      cert.Issuer,
		modDate:         time.Now(),
		serialNumber:    cert.SerialNumber.Bytes(),
	}
}

func (t trustItem) Serial() *big.Int {
	serial := big.NewInt(0)
	serial.SetBytes(t.serialNumber)
	return serial
}

func (t trustItem) String() string {
	modDate := t.modDate.Format(plistModDateFormat)

	name := fmt.Sprintf("O=%s", strings.Join(t.issuerName.Organization, " "))
	if t.issuerName.CommonName != "" {
		name = fmt.Sprintf("CN=%s", t.issuerName.CommonName)
	}

	country := strings.Join(t.issuerName.Country, " ")

	return fmt.Sprintf("SHA1 Fingerprint: %s\n %s (%s)\n modDate: %s\n serialNumber: %d", t.sha1Fingerprint, name, country, modDate, t.Serial())
}

func (t trustItem) equal(other trustItem) bool {
	return t.sha1Fingerprint == other.sha1Fingerprint
}

// parsePlist takes a reader of the xml output produced by trustSettingsExport()
// and converts it into a series of structs to then read
//
// After getting a `plist` callers will typically want to convert into
// a []trustItem by calling convertToTrustItems()
func parsePlist(in io.Reader) (plist, error) {
	dec := xml.NewDecoder(in)
	var out plist
	err := dec.Decode(&out)
	return out, err
}

// xml format, this was generated with the package github.com/gnewton/chidley
// but has also been modified by hand:
// 1. don't export struct names
// 2. remove outermost ChiChidleyRoot314159 wrapper as parsing fails with it
// 3. make `date []*date` rather than `date *date`
// 4. remove chi* from names as when we Marshal encoding/xml will use the struct's names
type plist struct {
	ChiDict *dict `xml:"dict,omitempty"`
}

type dict struct {
	ChiData    []*data    `xml:"data,omitempty"`
	ChiDate    []*date    `xml:"date,omitempty"`
	ChiDict    *dict      `xml:"dict,omitempty"`
	ChiInteger []*integer `xml:"integer,omitempty"`
	ChiKey     []*key     `xml:"key,omitempty"`
}

type key struct {
	Text string `xml:",chardata"`
}

type data struct {
	Text string `xml:",chardata"`
}

type date struct {
	Text string `xml:",chardata"`
}

type integer struct {
	Text bool `xml:",chardata"`
}

func (p plist) convertToTrustItems() trustItems {
	out := make([]trustItem, 0)

	max := len(p.ChiDict.ChiDict.ChiDict.ChiData)
	for i := 0; i < max; i += 2 {
		item := trustItem{}

		item.sha1Fingerprint = strings.ToLower(p.ChiDict.ChiDict.ChiKey[i/2].Text)

		// trim whitespace
		r := regexp.MustCompile(`[^a-zA-Z0-9\+\/=]*`)
		r2 := strings.NewReplacer("\t", "", "\n", "", " ", "", "\r", "")

		s1 := r2.Replace(r.ReplaceAllString(p.ChiDict.ChiDict.ChiDict.ChiData[i].Text, ""))
		s2 := r2.Replace(r.ReplaceAllString(p.ChiDict.ChiDict.ChiDict.ChiData[i+1].Text, ""))

		bs1, _ := base64.StdEncoding.DecodeString(s1)
		bs2, _ := base64.StdEncoding.DecodeString(s2)

		// The issuerName's <data></data> block is only under asn1 encoding for the
		// issuerName field from 4.1.2.4 (https://tools.ietf.org/rfc/rfc5280)
		var issuer pkix.RDNSequence
		_, err := asn1.Unmarshal(bs1, &issuer)
		if err == nil {
			name := pkix.Name{}
			name.FillFromRDNSequence(&issuer)
			item.issuerName = name
		}

		dt := p.ChiDict.ChiDict.ChiDict.ChiDate[i/2].Text
		t, err := time.ParseInLocation(plistModDateFormat, dt, time.UTC)
		if err == nil {
			item.modDate = t
		}

		// serialNumber is just a base64 encoded big endian (big) int
		item.serialNumber = bs2

		out = append(out, item)
	}

	return trustItems(out)
}
