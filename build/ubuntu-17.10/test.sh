#!/bin/bash
set -e

echo "Platform tests"
# Verify we're starting with the correct number of certs
/bin/cert-manage list -count | grep 148

# Make a backup
/bin/cert-manage backup

# Quick check
ls -1 /usr/share/ca-certificates/* | wc -l | grep 148
ls -1 /usr/share/ca-certificates.backup/* | wc -l | grep 148

# Whitelist and verify
/bin/cert-manage whitelist -file /whitelist.json
/bin/cert-manage list -count | grep 5

# Verify google.com fails to load
set +e
curl -v -I https://www.google.com/images/branding/product/ico/googleg_lodp.ico
code=$?
set -e
if [ "$code" -ne "35" ];
then
    echo "Got other status code from google.com request, code=$code"
    exit 1
fi

# Restore
/bin/cert-manage restore
/bin/cert-manage list -count | grep 148

# Verify google.com loads now
curl -v -I https://www.google.com/images/branding/product/ico/googleg_lodp.ico

## Chrome
echo "Chrome tests"
timeout 15s chromium-browser --no-sandbox --headless https://google.com 2>&1 >> /var/log/chrome.log
# /bin/cert-manage list -app chrome -count

## Firefox
echo "Firefox tests"
set +e
timeout 15s firefox --headless https://google.com 2>&1 >> /var/log/firefox.log
code=$?
if [ "$code" -ne "124" ];
then
  exit $code
fi
echo "firefox was forced to quit, code=$code"
set -e
count=$(/bin/cert-manage list -app firefox -count)
echo "Cert count from firefox: $count"
echo "$count" | grep -E 4

# Take a backup
[ ! -d ~/.cert-manage/firefox ]
/bin/cert-manage backup -app firefox
ls -1 ~/.cert-manage/firefox | wc -l | grep 1

# Whitelist
/bin/cert-manage whitelist -file /whitelist.json -app firefox
/bin/cert-manage list -app firefox -count | grep 1

# Restore that backup
for db in $(ls -1 ~/.mozilla/firefox/*.default/cert8.db | head -n1)
do
    # Force a difference we'd notice 5 a restore happens
    echo a > "$db"
    /bin/cert-manage restore -app firefox

    # Check we actaully restored a file
    size=$(stat --printf="%s" ~/.mozilla/firefox/*.default/cert8.db)
    if [ ! "$size" -gt "2" ];
    then
        echo "failed to restore firefox cert8.db properly"
        exit 1
    fi

    ls -l "$db"
done

# Verify restore
/bin/cert-manage list -app firefox -count | grep -E 4

# Java
echo "Java"

# Take a backup and verify
/bin/cert-manage list -app java -count | grep 148
/bin/cert-manage backup -app java
ls -1 ~/.cert-manage/java | wc -l | grep 1

# Verify google.com request loads
java Download

# Break the keystore
echo a > /usr/lib/jvm/java-9-openjdk-amd64/lib/security/cacerts

# Restore
/bin/cert-manage restore -app java
/bin/cert-manage list -app java -count | grep 148

# Verify restore
size=$(stat --printf="%s" /usr/lib/jvm/java-9-openjdk-amd64/lib/security/cacerts)
if [ ! "$size" -gt "2" ];
then
    echo "failed to restore java cacerts properly"
    exit 1
fi

/bin/cert-manage whitelist -file /whitelist.json -app java
/bin/cert-manage list -app java -count | grep 9

# Verify google.com request fails now that it should
set +e
out=$(java Download 2>&1)
set -e
if ! echo "$out" | grep 'PKIX path building failed';
then
    echo "Expected http response failure, but got something else, response:"
    echo "$out"
    exit 1
fi

echo "Ubuntu 17.10 Passed"