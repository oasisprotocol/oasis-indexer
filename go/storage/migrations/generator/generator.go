// Package generator generates migrations for the Oasis Indexer
// from the genesis file at a particular height.
package generator

import (
	"fmt"
	"os"
	"strings"

	genesis "github.com/oasisprotocol/oasis-core/go/genesis/api"
	staking "github.com/oasisprotocol/oasis-core/go/staking/api"

	"github.com/oasislabs/oasis-block-indexer/go/log"
)

const (
	chainID = "oasis_2"
)

// MigrationGenerator generates migrations for the Oasis Indexer
// target storage.
type MigrationGenerator struct {
	logger *log.Logger
}

// NewMigrationGenerator creates a new migration generator.
func NewMigrationGenerator(logger *log.Logger) *MigrationGenerator {
	return &MigrationGenerator{logger}
}

// WriteGenesisDocumentMigration writes the SQL migration for the genesis document
// into a file.
func (mg *MigrationGenerator) WriteGenesisDocumentMigration(filename string, document *genesis.Document) error {
	migration, err := mg.GenesisDocumentMigration(document)
	if err != nil {
		return err
	}

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(migration)
	if err != nil {
		return err
	}

	return nil
}

// GenesisDocumentMigration creates a new migration that re-initializes all
// height-dependent state as per the genesis state at that height.
func (mg *MigrationGenerator) GenesisDocumentMigration(document *genesis.Document) (string, error) {
	var b strings.Builder
	b.WriteString(`-- DO NOT MODIFY
-- This file was autogenerated by the oasis-indexer migration generator.

BEGIN;
`)

	if err := mg.addStakingBackendMigrations(&b, document.Staking); err != nil {
		return "", err
	}

	b.WriteString("\nCOMMIT;\n")

	return b.String(), nil
}

func (mg *MigrationGenerator) addStakingBackendMigrations(b *strings.Builder, document staking.Genesis) error {

	// Populate accounts.
	b.WriteString(fmt.Sprintf(`
-- Staking Backend Data
TRUNCATE %s.accounts CASCADE;`, chainID))
	b.WriteString(fmt.Sprintf(`
INSERT INTO %s.accounts (address, general_balance, nonce, escrow_balance_active, escrow_total_shares_active, escrow_balance_debonding, escrow_total_shares_debonding)
VALUES
`, chainID))

	i := 0
	for address, account := range document.Ledger {
		b.WriteString(fmt.Sprintf(
			"\t('%s', %d, %d, %d, %d, %d, %d)",
			address.String(),
			account.General.Balance.ToBigInt(),
			account.General.Nonce,
			account.Escrow.Active.Balance.ToBigInt(),
			account.Escrow.Active.TotalShares.ToBigInt(),
			account.Escrow.Debonding.Balance.ToBigInt(),
			account.Escrow.Debonding.TotalShares.ToBigInt(),
		))
		i++

		if i != len(document.Ledger) {
			b.WriteString(",\n")
		}
	}
	b.WriteString(";\n")

	// Populate allowances.
	b.WriteString(fmt.Sprintf(`
TRUNCATE %s.allowances CASCADE;`, chainID))

	foundAllowances := false // in case allowances are empty

	i = 0
	for owner, account := range document.Ledger {
		if len(account.General.Allowances) > 0 && !foundAllowances {
			b.WriteString(fmt.Sprintf(`
INSERT INTO %s.allowances (owner, beneficiary, allowance)
VALUES
`, chainID))
			foundAllowances = true
		}

		rows := make([]string, 0, len(account.General.Allowances))
		j := 0
		for beneficiary, allowance := range account.General.Allowances {
			rows[j] = fmt.Sprintf(
				"\t('%s', '%s', %d)",
				owner.String(),
				beneficiary.String(),
				allowance.ToBigInt(),
			)
			j++
		}
		b.WriteString(strings.Join(rows, ",\n"))
		i++

		if i != len(document.Ledger) {
			b.WriteString(",\n")
		}
	}
	if foundAllowances {
		b.WriteString(";\n")
	}

	// Populate delegations.
	b.WriteString(fmt.Sprintf(`
TRUNCATE %s.delegations CASCADE;`, chainID))
	b.WriteString(fmt.Sprintf(`
INSERT INTO %s.delegations (delegatee, delegator, shares)
VALUES
`, chainID))
	i = 0
	for delegatee, escrows := range document.Delegations {
		delegateeDelegations := make([]string, len(escrows))
		j := 0
		for delegator, delegation := range escrows {
			delegateeDelegations[j] = fmt.Sprintf(
				"\t('%s', '%s', %d)",
				delegatee.String(),
				delegator.String(),
				delegation.Shares.ToBigInt(),
			)
			j++
		}
		b.WriteString(strings.Join(delegateeDelegations, ",\n"))
		i++

		if i != len(document.Delegations) {
			b.WriteString(",\n")
		}
	}
	b.WriteString(";\n")

	// Populate debonding delegations.
	b.WriteString(fmt.Sprintf(`
TRUNCATE %s.debonding_delegations CASCADE;`, chainID))
	b.WriteString(fmt.Sprintf(`
INSERT INTO %s.debonding_delegations (delegatee, delegator, shares, debond_end)
VALUES
`, chainID))
	i = 0
	for delegatee, escrows := range document.DebondingDelegations {
		delegateeDebondingDelegations := make([]string, 0)
		j := 0
		for delegator, debondingDelegations := range escrows {
			delegatorDebondingDelegations := make([]string, len(debondingDelegations))
			for k, debondingDelegation := range debondingDelegations {
				delegatorDebondingDelegations[k] = fmt.Sprintf(
					"\t('%s', '%s', %d, %d)",
					delegatee.String(),
					delegator.String(),
					debondingDelegation.Shares.ToBigInt(),
					debondingDelegation.DebondEndTime,
				)
			}
			delegateeDebondingDelegations = append(delegateeDebondingDelegations, delegatorDebondingDelegations...)
			j++
		}
		b.WriteString(strings.Join(delegateeDebondingDelegations, ",\n"))
		i++

		if i != len(document.DebondingDelegations) {
			b.WriteString(",\n")
		}
	}
	b.WriteString(";\n")

	return nil
}
