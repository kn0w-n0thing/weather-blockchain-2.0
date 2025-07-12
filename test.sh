#!/bin/bash

# Check for clean argument
if [ "$1" == "clean" ]; then
    echo "Cleaning up node directories..."

    # Clean node-1
    if [ -d "test/node-1/logs" ]; then
        rm -rf test/node-1/logs
        echo "Removed test/node-1/logs"
    fi
    if [ -d "test/node-1/data" ]; then
        rm -rf test/node-1/data
        echo "Removed test/node-1/data"
    fi

    # Clean node-2
    if [ -d "test/node-2/logs" ]; then
        rm -rf test/node-2/logs
        echo "Removed test/node-2/logs"
    fi
    if [ -d "test/node-2/data" ]; then
        rm -rf test/node-2/data
        echo "Removed test/node-2/data"
    fi

    # Clean node-3
    if [ -d "test/node-3/logs" ]; then
        rm -rf test/node-3/logs
        echo "Removed test/node-3/logs"
    fi
    if [ -d "test/node-3/data" ]; then
        rm -rf test/node-3/data
        echo "Removed test/node-3/data"
    fi

    echo "Cleanup completed."
    exit 0
fi

# Build the main binary
if ! go build main.go; then
    echo "Error: Failed to build main.go"
    exit 1
fi

# Copy binary to test directories
cp main test/node-1/
cp main test/node-2/
cp main test/node-3/

# Store the root directory
root_dir=$(pwd)

# Start node 1 (genesis node)
cd test/node-1
./main --port 10001 --genesis &
node1_pid=$!

# Start node 2
cd "$root_dir/test/node-2"
./main --port 10002 &
node2_pid=$!

# Start node 3
cd "$root_dir/test/node-3"
./main --port 10003 &
node3_pid=$!

# Print PIDs for manual cleanup
echo "Started nodes with PIDs: $node1_pid, $node2_pid, $node3_pid"

# Optional: Add cleanup function
cleanup() {
    echo "Stopping nodes..."
    kill $node1_pid $node2_pid $node3_pid 2>/dev/null
    cd "$root_dir"
}

# Optional: Trap to cleanup on script exit
trap cleanup EXIT INT TERM

# Keep script running (optional)
wait