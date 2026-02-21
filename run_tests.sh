#!/bin/bash

echo "=== TEST GROUP 1: REST API & FILE UPLOAD ==="
echo "--> Test 1.1: Basic File Upload"
RESP=$(curl -s -i -F "file=@sample.txt" http://localhost:8080/files)
echo "$RESP" | grep "HTTP/1.1"
echo "$RESP" | grep "{"
ID=$(echo "$RESP" | grep -oE '"id":"[a-f0-9-]+"' | cut -d'"' -f4)

echo ""
echo "--> Test 1.2: GET File Metadata ($ID)"
sleep 1 # Wait for processing
curl -s "http://localhost:8080/files/$ID" | python3 -m json.tool

echo ""
echo "--> Test 1.3: Invalid Request Handling"
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8080/files)
echo "HTTP Status: $CODE (Expected: 400)"

echo ""
echo "=== TEST GROUP 2: WORKER POOL & CONCURRENCY ==="
echo "--> Test 2.1: Async Processing Verification"
cat server.log | grep "$ID" | head -n 3
echo "(Check timestamps: Upload request received vs Processing completed)"

echo ""
echo "--> Test 2.2: Multiple Upload Stress Test (10 requests)"
for i in {1..10}; do
  curl -s -F "file=@sample.txt" http://localhost:8080/files > /dev/null &
done
wait
echo "Stress test queued."
sleep 2 # Let workers work
echo "Active Workers Log Sample:"
grep "processing started" server.log | tail -n 5

echo ""
echo "--> Test 2.3: Hash Correctness"
LOCAL_HASH=$(shasum -a 256 sample.txt | awk '{print $1}')
REMOTE_HASH=$(curl -s "http://localhost:8080/files/$ID" | grep '"hash":' | cut -d'"' -f4)
echo "Local SHA256:  $LOCAL_HASH"
echo "Remote SHA256: $REMOTE_HASH"

echo ""
echo "=== TEST GROUP 3: DATABASE ==="
echo "--> Test 3.1 & 3.2: Metadata Updates"
echo "Database Record:"
mysql -u root -D gopherdrive -e "SELECT id, status, hash, size FROM files WHERE id='$ID';"

echo ""
echo "=== TEST GROUP 4: FILE HANDLING ==="
echo "--> Test 4.1: UUID Naming"
ls -l data/

echo ""
echo "--> Test 4.2: Large File Upload (10MB)"
curl -s -o /dev/null -w "Uploaded: %{http_code}\n" -F "file=@large_file.bin" http://localhost:8080/files

echo ""
echo "=== TEST GROUP 5: PRODUCTION READINESS ==="
echo "--> Test 5.2: Health Check"
curl -s http://localhost:8080/healthz | python3 -m json.tool

echo ""
echo "--> Test 5.1: Graceful Shutdown"
# We'll simulate this by killing the server process but capturing logs
PID=$(lsof -ti:8080)
if [ ! -z "$PID" ]; then
  kill -SIGINT $PID
  echo "Sent SIGINT to PID $PID"
  sleep 2
  tail -n 10 server.log | grep "shutdown"
else
  echo "Server not running?"
fi
