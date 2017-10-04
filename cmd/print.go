package cmd

import (
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/adamdecaf/cert-manage/tools/_x509"
)

const (
	fingerprintPreviewLength = 16
)

// PrintCerts outputs the slice of certificates in `format` to stdout
// Format can be 'table' and any other value will output them in more detail
func printCerts(certs []*x509.Certificate, format string) {
	if format == "table" {
		printCertsInTable(certs)
	} else {
		printCertsToStdout(certs)
	}
}

// printCertsInTable outputs a nicely formatted table of the certs found. This uses golang's
// native text/tabwriter package to align based on the rows given to it.
func printCertsInTable(certs []*x509.Certificate) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(w, "Subject\tIssuer\tPublic Key Algorithm\tFingerprint\tNot Before\tNot After")
	defer func() {
		err := w.Flush()
		if err != nil {
			fmt.Printf("error flushing output table - %s\n", err)
		}
	}()

	rows := make([]string, len(certs))
	for i := range certs {
		fingerprint := _x509.GetHexSHA256Fingerprint(*certs[i])

		c1 := certs[i].Subject.CommonName
		c2 := certs[i].Issuer.CommonName
		c3 := _x509.StringifyPubKeyAlgo(certs[i].PublicKeyAlgorithm)
		c4 := fingerprint[:fingerprintPreviewLength]

		c5 := certs[i].NotBefore.Format("2006-01-02")
		c6 := certs[i].NotAfter.Format("2006-01-02")

		rows[i] = fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s", c1, c2, c3, c4, c5, c6)
	}

	sort.Sort(iStringSlice(rows))
	for i := range rows {
		fmt.Fprintln(w, rows[i])
	}
}

// printCertsToStdout very verbosly prints out the ecah certificate's information
// to stdout. This isn't very useful for machine parsing or small screen displays.
func printCertsToStdout(certs []*x509.Certificate) {
	for i := range certs {
		fingerprint := _x509.GetHexSHA256Fingerprint(*certs[i])

		fmt.Printf("Certificate\n")
		fmt.Printf("  SHA256 Fingerprint - %s\n", fingerprint)
		fmt.Printf("  Signature - %s\n", hex.EncodeToString(certs[i].Signature))
		fmt.Printf("  Signature Algorithm: %s\n", certs[i].SignatureAlgorithm.String())
		fmt.Printf("  Public Key Algorithm - %v\n", _x509.StringifyPubKeyAlgo(certs[i].PublicKeyAlgorithm))
		fmt.Printf("  Issuer CommonName - %s, SerialNumber - %s\n", certs[i].Issuer.CommonName, certs[i].Issuer.SerialNumber)
		fmt.Printf("  Subject CommonName - %s, SerialNumber - %s\n", certs[i].Subject.CommonName, certs[i].Subject.SerialNumber)
		fmt.Printf("  NotBefore - %s, NotAfter - %s\n", certs[i].NotBefore, certs[i].NotAfter)
		fmt.Printf("  IsCA - %t\n", certs[i].IsCA)
		fmt.Printf("  MaxPathLen - %d\n", certs[i].MaxPathLen)

		fmt.Printf("  DNSNames\n")
		for j := range certs[i].DNSNames {
			fmt.Printf("    %s\n", certs[i].DNSNames[j])
		}

		fmt.Printf("  EmailAddresses\n")
		for j := range certs[i].EmailAddresses {
			fmt.Printf("    %s\n", certs[i].EmailAddresses[j])
		}

		fmt.Printf("  IPAddresses\n")
		for j := range certs[i].IPAddresses {
			fmt.Printf("    %s\n", certs[i].IPAddresses[j])
		}

		fmt.Printf("  PermittedDNSDomains\n")
		for j := range certs[i].PermittedDNSDomains {
			fmt.Printf("    %s\n", certs[i].PermittedDNSDomains[j])
		}

		fmt.Printf("  CRLDistributionPoints\n")
		for j := range certs[i].CRLDistributionPoints {
			fmt.Printf("    %s\n", certs[i].CRLDistributionPoints[j])
		}
	}
}

// IStringSlice is a case-insensitive string sorting implementation
type iStringSlice []string

func (p iStringSlice) Len() int           { return len(p) }
func (p iStringSlice) Less(i, j int) bool { return strings.ToLower(p[i]) < strings.ToLower(p[j]) }
func (p iStringSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
