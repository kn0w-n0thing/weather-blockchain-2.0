package block

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"
	"weather-blockchain/account"
)

const PrevHashOfGenesis = "0000000000000000000000000000000000000000000000000000000000000000"

type Block struct {
	Index            uint64
	Timestamp        int64
	PrevHash         string
	ValidatorAddress string
	Data             string
	Signature        []byte
	Hash             string
}

func (block *Block) CalculateHash() []byte {
	// Create a string containing all fields that should contribute to the hash
	// Note: We don't include the Signature or Hash fields, as those are derived values
	record := strconv.FormatUint(block.Index, 10) +
		strconv.FormatInt(block.Timestamp, 10) +
		block.PrevHash +
		block.ValidatorAddress +
		block.Data

	// Hash the record using SHA-256
	sha := sha256.New()
	sha.Write([]byte(record))
	hash := sha.Sum(nil)

	return hash
}

func CreateGenesisBlock(creatorAccount *account.Account) (*Block, error) {
	genesisBlock := &Block{
		Index:            0,
		Timestamp:        time.Now().Unix(),
		PrevHash:         PrevHashOfGenesis,
		ValidatorAddress: creatorAccount.Address,
		Data:             "Genesis Block",
	}

	// Sign the block
	signature, err := creatorAccount.Sign(genesisBlock.CalculateHash())
	if err != nil {
		return nil, err
	}
	genesisBlock.Signature = signature

	return genesisBlock, nil
}

// StoreHash calculates and stores the hash in the block
func (block *Block) StoreHash() {
	hashBytes := block.CalculateHash()
	block.Hash = hex.EncodeToString(hashBytes)
}
