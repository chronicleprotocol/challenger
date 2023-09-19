// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.17;

// forge create src/MockScribeOptimistic.sol:MockScribeOptimistic

contract MockScribeOptimistic {
    bool public acceptSig;
    bool public challengeSuccessful;

    uint16 internal _opchallengePeriod;

    constructor() {
        _opchallengePeriod = 599;
        acceptSig = false; // An invalid signature is the trigger for a challenge
        challengeSuccessful = true;
    }

    event Generic(address from);
    function setAcceptSig(bool ok) external {
        acceptSig = ok;
        emit Generic(msg.sender);
    }

    function setChallengeSuccessful(bool ok) external {
        challengeSuccessful = ok;
    }

    struct SchnorrData {
        bytes32 signature;
        address commitment;
        bytes signersBlob;
    }

    struct PokeData {
        uint128 val;
        uint32 age;
    }

    struct ECDSAData {
        uint8 v;
        bytes32 r;
        bytes32 s;
    }

    event OpPoked(
        address indexed caller,
        address indexed opFeed,
        SchnorrData schnorrData,
        PokeData pokeData
    );

    function setOpChallengePeriod(uint16 val) external {
        _opchallengePeriod = val;
    }

    function opChallengePeriod() public view returns (uint16) {
        return _opchallengePeriod;
    }

    function opPoke(
        PokeData calldata pokeData,
        SchnorrData calldata schnorrData,
        ECDSAData calldata /*ecdsaData*/
    ) external {
        address signer = 0x15d34AAf54267DB7D7c367839AAf71A00a2C6A65;
        emit OpPoked(msg.sender, signer, schnorrData, pokeData);
    }

    function constructPokeMessage(
        PokeData calldata /*pokeData*/
    ) external pure returns (bytes32) {
        return 0x77617468656c6f77726c645f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f;
    }

    function isAcceptableSchnorrSignatureNow(
        bytes32 /*message*/,
        SchnorrData calldata /*schnorrData*/
    ) external view returns (bool) {
        return acceptSig;
    }

    function opChallenge(SchnorrData calldata /*schnorrData*/)
        external
        view
        returns (bool)
    {
        return challengeSuccessful; 
    }
}
