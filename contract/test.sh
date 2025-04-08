#!/usr/bin/env bash

gitroot=$(git rev-parse --show-toplevel)
me=$(basename $0)

function cecho {
    echo "# $@"
}

[[ -z "$1" ]] && {
    echo "
Usage: $me <contract address> [poke|sigreject|sigaccept|challfails|challsucceeds|info]
    poke            Send a poke
    sigreject       Set contract to reject signature on isAcceptableSchnorrSignatureNow() - will trigger challenge
    sigaccept       Set contract to accept signature on isAcceptableSchnorrSignatureNow()
    challfails      Set contract to fail on opChallenge()
    challsucceeds   Set contract to succeed on opChallenge()

    NOTE if you do not provide one of the above params, it will run a full integration test.
"
    exit 1
}

contract="$1"

function showinfo {
    echo "${contract}.opChallengePeriod:   $(cast call $contract 'opChallengePeriod()(uint16)')"
    echo "${contract}.acceptSig:           $(cast call $contract 'acceptSig()(bool)')"
    echo "${contract}.challengeSuccessful: $(cast call $contract 'challengeSuccessful()(bool)')"
}

function do_poke {
    cecho "Sending opPoke -> $contract ..."
    cast send $contract \
"opPoke((uint128,uint32),(bytes32,address,bytes),(uint8,bytes32,bytes32))" \
"(1,1694714738)" \
"(0x1111111111111111111111111111111111111111111111111111111111111111,0xa0Ee7A142d267C1f36714E4a8F75612F20a79720,0x2222222222222222222222222222222222222222222222222222222222222222)" \
"(27,0x3333333333333333333333333333333333333333333333333333333333333333,0x4444444444444444444444444444444444444444444444444444444444444444)"
}

function do_sigreject {
    cecho "Sending setAcceptSig(false) -> $contract ... (will trigger opChallenge)"
    cast send $contract 'setAcceptSig(bool)' 0
    echo "${contract}.acceptSig: $(cast call $contract 'acceptSig()(bool)')"
}

function do_sigaccept {
    cecho "Sending setAcceptSig(true) -> $contract ..."
    cast send $contract 'setAcceptSig(bool)' 1
    echo "${contract}.acceptSig: $(cast call $contract 'acceptSig()(bool)')"
}

if [ "$2" = "poke" ]; then
    do_poke
    exit 0
fi

if [ "$2" = "sigreject" ]; then
    do_sigreject
    exit 0
fi

if [ "$2" = "sigaccept" ]; then
    do_sigaccept
    exit 0
fi

if [ "$2" = "challsucceeds" ]; then
    cecho "Sending setChallengeSuccessful(true) -> $contract ..."
    cast send $contract 'setChallengeSuccessful(bool)' 1
    echo "${contract}.challengeSuccessful: $(cast call $contract 'challengeSuccessful()(bool)')"
    exit 0
fi

if [ "$2" = "challfails" ]; then
    cecho "Sending setChallengeSuccessful(false) -> $contract ..."
    cast send $contract 'setChallengeSuccessful(bool)' 0
    echo "${contract}.challengeSuccessful: $(cast call $contract 'challengeSuccessful()(bool)')"
    exit 0
fi

if [ "$2" = "info" ]; then
    showinfo
    exit 0
fi

chainid=11155111 # Sepolia
gocmd=go
[[ ! -z "$GOCMD" ]] && gocmd=$GOCMD

function must_env {
    env_var=$1
    if [ -z "${!env_var}" ]; then
        cecho "$1 env var must be set!"
        exit 1
    fi
}

function end_challenger {
    pkill -f challenger-test
}

function start_challenger {
    must_env ETH_RPC_URL
    must_env ETH_WSS_URL
    must_env ETH_KEYSTORE_FILE
    must_env ETH_PASS

    end_challenger

    cd "$gitroot"

    rm -f /tmp/challenger-test
    $gocmd build -o /tmp/challenger-test cmd/challenger/*.go
    (/tmp/challenger-test run \
        -a $contract \
        --rpc-url "$ETH_RPC_URL" \
        --keystore "$ETH_KEYSTORE_FILE" \
        --password "$ETH_PASS" \
        --chain-id "$chainid" \

    sleep 5
    cecho "Challenger running"
}

if [ -z "$2" ]; then # Integration test
    start_challenger
    sleep 5
    showinfo
    do_poke
    sleep 10
    end_challenger
else
    echo "Not sure what to do with: $2"
    exit 1
fi
