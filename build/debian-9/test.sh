#!/bin/bash
set -e

cd build/debian-9/
cp ../../bin/cert-manage-linux-amd64 cert-manage
cp ../../testdata/globalsign-whitelist.json whitelist.json

total=167
after=6
cat > main <<EOF
#!/bin/sh
set -e

# Verify we're starting with the correct number of certs
/bin/cert-manage -list | wc -l | grep $total

# Make a backup
/bin/cert-manage -backup

# Quick check
ls -l /usr/share/ca-certificates/* | wc -l | grep $total
ls -l /usr/share/ca-certificates.backup/* | wc -l | grep $total

# Whitelist and verify
/bin/cert-manage -whitelist /whitelist.json
/bin/cert-manage -list | wc -l | grep $after

# Restore
/bin/cert-manage -restore
/bin/cert-manage -list | wc -l | grep $total
EOF

chmod +x main
docker build -t cert-manage-debian-9:latest . 2>&1 > test.log
docker run -i --entrypoint /bin/main cert-manage-debian-9:latest 2>&1 >> test.log