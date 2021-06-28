#!/bin/bash
set -e

# Set up the routing needed for the simulation.
/setup.sh

if [ "$ROLE" == "sender" ]; then
    # Wait for the simulator to start up.
    /wait-for-it.sh sim:57832 -s -t 10
    echo "Starting RTQ sender..."
    echo "Sender params: $SENDER_PARAMS"
    echo "Test case: $TESTCASE"
    QUIC_GO_LOG_LEVEL=debug ./sender $SENDER_PARAMS $VIDEOS
else
    echo "Running RTQ receiver."
    QUIC_GO_LOG_LEVEL=debug ./receiver "$@"
fi