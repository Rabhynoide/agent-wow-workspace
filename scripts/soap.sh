#!/usr/bin/env bash
# usage: scripts/soap.sh '<GM command>'   (e.g. 'server info', 'pdump write snap6 Benchabcdef')
# Sends a GM command to the worldserver over SOAP (127.0.0.1:7878, GM account AC_SOAP_USER /
# AC_SOAP_PASS in .env) and prints the answer. Exit 1 when the server reports a failure.
set -u
cd "$(dirname "$0")/.."
user=$(grep '^AC_SOAP_USER=' .env | cut -d= -f2-)
pass=$(grep '^AC_SOAP_PASS=' .env | cut -d= -f2-)
cmd=$(python3 -Ic 'import sys, html; print(html.escape(sys.argv[1]))' "$1")
body="<?xml version=\"1.0\" encoding=\"utf-8\"?><SOAP-ENV:Envelope xmlns:SOAP-ENV=\"http://schemas.xmlsoap.org/soap/envelope/\" xmlns:ns1=\"urn:AC\"><SOAP-ENV:Body><ns1:executeCommand><command>$cmd</command></ns1:executeCommand></SOAP-ENV:Body></SOAP-ENV:Envelope>"
out=$(curl -s --max-time 30 -u "$user:$pass" -H 'Content-Type: text/xml' --data-binary "$body" http://127.0.0.1:7878/) || { echo "soap: no answer"; exit 1; }
python3 -Ic '
import sys, re, html
s = sys.argv[1]
m = re.search(r"<result>(.*?)</result>", s, re.S) or re.search(r"<faultstring>(.*?)</faultstring>", s, re.S)
print(html.unescape(m.group(1)).strip() if m else s.strip())
sys.exit(0 if "<result>" in s else 1)' "$out"
