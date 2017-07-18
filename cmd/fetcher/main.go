package main

import (
	"crypto/x509"
	// "encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"github.com/adamdecaf/cert-manage/cmd"
	"github.com/adamdecaf/cert-manage/fetch/ca"
	ver "github.com/adamdecaf/cert-manage/version"
)

var (
	// file = flag.String("file", "", "Whitelist output file location")

	// TODO(adam): switch default to false when we add json whitelist writing back in
	print = flag.Bool("print", true, "Print the certs that will be put into the whitelist json")

	version = flag.Bool("version", false, "Output the version information")
)

func main() {
	flag.Parse()

	if set(version) {
		fmt.Printf("cert-manage fetcher: %s\n", ver.Version)
		return
	}

	// Get the CAs to grab certs for
	cas := flag.Args()

	// accumulators
	m := sync.Mutex{}
	whitelisted := make([]*x509.Certificate, 0)
	errors := make([]error, 0)

	wg := sync.WaitGroup{}
	wg.Add(len(cas))

	add := func(cs []*x509.Certificate, err error) {
		m.Lock()
		whitelisted = append(whitelisted, cs...)
		if err != nil {
			errors = append(errors, err)
		}
		m.Unlock()
	}

	for i := range cas {
		go func(who string) {
			defer wg.Done()

			switch strings.TrimSpace(strings.ToLower(who)) {
			case "android":
				add(ca.Android())
			case "apple":
				add(ca.Apple())
			case "ct":
				add(ca.CT())
			case "darwin":
				add(ca.Darwin())
			case "digicert":
				add(ca.Digicert())
			case "google":
				add(ca.Google())
			case "java":
				add(ca.Java())
			case "linux":
				add(ca.Linux())
			case "microsoft":
				add(ca.Microsoft())
			case "nss":
				add(ca.NSS())
			case "visa":
				add(ca.Visa())
			case "windows":
				add(ca.Windows())
			}

		}(cas[i])
	}

	wg.Wait()

	exit := 0

	// Print any errors
	if len(errors) > 0 {
		exit = 1
	}
	for _, err := range errors {
		fmt.Println(err)
	}

	// Print certs
	if set(print) {
		if len(whitelisted) > 0 {
			cmd.PrintCerts(whitelisted, "table")
		} else {
			exit = 1
			fmt.Println("No certificates found")
		}
	}

	// TODO(adam): write whitelist json file

	os.Exit(exit)
}

func set(b *bool) bool {
	return b != nil && *b
}