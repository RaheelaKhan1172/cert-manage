package test

import (
	"fmt"
	"testing"
)

func linuxSuite(t *testing.T, img *dockerfile, total, after string) {
	if debug {
		fmt.Println("Linux start")
	}

	// List
	img.CertManage("list", "-count", "|", "grep", total)
	// Backup
	img.CertManage("backup")
	img.RunSplit(fmt.Sprintf("ls -1 /usr/share/ca-certificates/* | wc -l | grep %s", total))
	img.RunSplit(fmt.Sprintf("ls -1 /usr/share/ca-certificates.backup/* | wc -l | grep %s", total))
	// Whitelist
	img.CertManage("whitelist", "-file", "/whitelist.json")
	img.CertManage("list", "-count", "|", "grep", after)
	// Restore
	img.CertManage("restore")
	img.CertManage("list", "-count", "|", "grep", total)
	img.SuccessT(t)

	if debug {
		fmt.Println("Linux end")
	}
}

func javaSuite(t *testing.T, img *dockerfile, total, after string) {
	if debug {
		fmt.Println("Java start")
	}

	// List
	img.CertManage("list", "-count", "-app", "java", "|", "grep", total)
	// Backup
	img.CertManage("backup", "-app", "java")
	img.RunSplit("ls -1 ~/.cert-manage/java | wc -l | grep 1")
	// Check java
	img.RunSplit("cd / && java Download")
	// Break java
	// img.RunSplit("echo a > /usr/lib/jvm/java-8-openjdk-amd64/jre/lib/security/cacerts")     // TODO
	// img.RunSplit("cd / && java Download") // Verify this fails, 'PKIX path building failed' // TODO
	// Restore
	img.CertManage("restore", "-app", "java")
	img.CertManage("list", "-app", "java", "-count", "|", "grep", total)
	// Verify restore
	// size=$(stat --printf="%s" /usr/lib/jvm/java-8-openjdk-amd64/jre/lib/security/cacerts)   // TODO
	img.CertManage("whitelist", "-file", "/whitelist.json", "-app", "java")
	img.CertManage("list", "-app", "java", "-count", "|", "grep", after)
	// Verify google.com fails to load
	// out=$(java Download 2>&1); if ! echo "$out" | grep 'PKIX path building failed';         // TODO

	img.SuccessT(t)

	if debug {
		fmt.Println("Java end")
	}
}