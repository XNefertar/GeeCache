#!/bin/bash
set -e

# Get the directory of the script
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$DIR/../.."

# Ensure we are at project root
cd "$PROJECT_ROOT"

echo "Generating Go code from proto..."

# Clean old generated files
rm -f go/geecachepb/*.pb.go

# Generate new files
# --go_out=go: Output to go/ directory
# --go_opt=module=geecache: Strip 'geecache' prefix from the output path, so geecache/geecachepb -> go/geecachepb
protoc --proto_path=proto \
       --go_out=go --go_opt=module=geecache \
       --go-grpc_out=go --go-grpc_opt=module=geecache \
       proto/geecache.proto

echo "Done."
