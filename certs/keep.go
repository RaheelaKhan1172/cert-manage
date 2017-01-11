package certs

import (
	"crypto/x509"
)

// Keep filters a list of x509 Certificates against whitelist items to
// retain only the certificates that are allowed by our whitelist.
// An empty slice of certificates is a possible (and valid) output.
func Keep(incoming []*x509.Certificate, whitelisted []WhitelistItem) []*x509.Certificate {
	// Pretty bad search right now.
	var keep []*x509.Certificate
	for _,inc := range incoming {
		for _,wh := range whitelisted {
			if inc != nil && wh.Matches(*inc) {
				keep = append(keep, inc)
			}
		}
	}
	return keep
}

// todo: dedup certs already added by one whitelist item
// e.g. If my []WhitelistItem contains a signature and Issuer.CommonName match
// don't add the cert twice