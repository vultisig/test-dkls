#!/bin/bash
session=$RANDOM

echo "Migrate ECDSA key, session: $session"
# first party
./test-dkls --key first  --session $session --leader migrate --file GG20-silencelab-three-parties-2d33-part1of3.json &

# second party
./test-dkls --key second  --session $session  migrate --file GG20-silencelab-three-parties-2d33-part2of3.json &

# third party

./test-dkls --key third  --session $session  migrate --file GG20-silencelab-three-parties-2d33-part3of3.json  &

wait

session=$RANDOM

echo "Generating EdDSA key, session: $session"

# first party
./test-dkls --key first --session $session --leader migrate --file GG20-silencelab-three-parties-2d33-part1of3.json --eddsa &

# second party
./test-dkls --key second --session $session  migrate --file GG20-silencelab-three-parties-2d33-part2of3.json --eddsa &

# third party

./test-dkls --key third  --session $session  migrate --file GG20-silencelab-three-parties-2d33-part3of3.json --eddsa &

wait
