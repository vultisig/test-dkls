#!/bin/bash
session=$RANDOM

echo "Generating ECDSA key, session: $session"
# first party
./test-dkls --key first --parties first,second,third --session $session --leader keygen --chaincode a983b1cb3143e5c946ab95fc7694f33f1935cca4a51ac683666b59bde0c339bc &

# second party
./test-dkls --key second --parties first,second,third --session $session  keygen --chaincode a983b1cb3143e5c946ab95fc7694f33f1935cca4a51ac683666b59bde0c339bc &

# third party

./test-dkls --key third --parties first,second,third --session $session  keygen --chaincode a983b1cb3143e5c946ab95fc7694f33f1935cca4a51ac683666b59bde0c339bc &

wait

session=$RANDOM

echo "Generating EdDSA key, session: $session"

# first party
./test-dkls --key first --parties first,second,third --session $session --leader keygen --chaincode a983b1cb3143e5c946ab95fc7694f33f1935cca4a51ac683666b59bde0c339bc --eddsa &

# second party
./test-dkls --key second --parties first,second,third --session $session  keygen --chaincode a983b1cb3143e5c946ab95fc7694f33f1935cca4a51ac683666b59bde0c339bc --eddsa &

# third party

./test-dkls --key third --parties first,second,third --session $session  keygen --chaincode a983b1cb3143e5c946ab95fc7694f33f1935cca4a51ac683666b59bde0c339bc --eddsa &

wait
