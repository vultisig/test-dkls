#!/bin/bash

# Check if the public key argument is provided
if [ -z "$1" ]; then
  echo "Usage: $0 <public_key>"
  exit 1
fi
session=$RANDOM
pubkey=$1
echo "Resharing ECDSA key,add fourth party, session: $session"
# first party
./test-dkls --key first --parties first,second,third,fourth --session $session --leader reshare --pubkey $pubkey --old-parties first,second,third &
# second party
./test-dkls --key second --parties first,second,third,fourth --session $session reshare --pubkey $pubkey --old-parties first,second,third &

# third party
./test-dkls --key third --parties first,second,third,fourth --session $session reshare --pubkey $pubkey --old-parties first,second,third &

# fourth party - the new guy
./test-dkls --key fourth --parties first,second,third,fourth --session $session reshare --pubkey '' --old-parties first,second,third &

wait

session=$RANDOM
echo "Resharing EdDSA key,remove fourth party, session: $session"
# first party
./test-dkls --key first --parties first,second,third --session $session --leader reshare --pubkey $pubkey --old-parties first,second,third &
# second party
./test-dkls --key second --parties first,second,third --session $session reshare --pubkey $pubkey --old-parties first,second,third &

# third party
./test-dkls --key third --parties first,second,third --session $session reshare --pubkey $pubkey --old-parties first,second,third &

wait