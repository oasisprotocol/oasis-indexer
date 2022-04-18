// Package generator generates migrations for the Oasis Indexer
// from the genesis file at a particular height.
package generator

import (
	"fmt"
	"os"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/oasisprotocol/oasis-core/go/common/entity"
	"github.com/oasisprotocol/oasis-core/go/common/node"
	genesis "github.com/oasisprotocol/oasis-core/go/genesis/api"
	registry "github.com/oasisprotocol/oasis-core/go/registry/api"

	"github.com/oasislabs/oasis-block-indexer/go/log"
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
// height-dependent state as per the provided genesis document.
func (mg *MigrationGenerator) GenesisDocumentMigration(document *genesis.Document) (string, error) {
	var b strings.Builder
	b.WriteString(`-- DO NOT MODIFY
-- This file was autogenerated by the oasis-indexer migration generator.

BEGIN;
`)

	for _, f := range []func(*strings.Builder, *genesis.Document) error{
		mg.addRegistryBackendMigrations,
		mg.addStakingBackendMigrations,
		mg.addGovernanceBackendMigrations,
	} {
		if err := f(&b, document); err != nil {
			return "", err
		}
	}

	b.WriteString("\nCOMMIT;\n")

	return b.String(), nil
}

func (mg *MigrationGenerator) addRegistryBackendMigrations(b *strings.Builder, document *genesis.Document) error {
	chainID := strcase.ToSnake(document.ChainID)

	// Populate entities.
	b.WriteString(fmt.Sprintf(`
-- Registry Backend Data
TRUNCATE %s.entities CASCADE;`, chainID))
	b.WriteString(fmt.Sprintf(`
INSERT INTO %s.entities (id)
VALUES
`, chainID))
	for i, signedEntity := range document.Registry.Entities {
		var entity entity.Entity
		if err := signedEntity.Open(registry.RegisterEntitySignatureContext, &entity); err != nil {
			return err
		}

		b.WriteString(fmt.Sprintf(
			"\t('%s')",
			entity.ID.String(),
		))

		if i != len(document.Registry.Entities)-1 {
			b.WriteString(",\n")
		}
	}
	b.WriteString(";\n")

	// Populate nodes.
	b.WriteString(fmt.Sprintf(`
TRUNCATE %s.nodes CASCADE;`, chainID))
	b.WriteString(fmt.Sprintf(`
INSERT INTO %s.nodes (id, entity_id, expiration, tls_pubkey, tls_next_pubkey, p2p_pubkey, consensus_pubkey, roles)
VALUES
`, chainID))
	for i, signedNode := range document.Registry.Nodes {
		var node node.Node
		if err := signedNode.Open(registry.RegisterNodeSignatureContext, &node); err != nil {
			return err
		}

		b.WriteString(fmt.Sprintf(
			"\t('%s', '%s', %d, '%s', '%s', '%s', '%s', '%s')",
			node.ID.String(),
			node.EntityID.String(),
			node.Expiration,
			node.TLS.PubKey.String(),
			node.TLS.NextPubKey.String(),
			node.P2P.ID.String(),
			node.Consensus.ID.String(),
			node.Roles.String(),
		))

		if i != len(document.Registry.Nodes)-1 {
			b.WriteString(",\n")
		}
	}
	b.WriteString(";\n")

	// Populate runtimes.
	b.WriteString(fmt.Sprintf(`
TRUNCATE %s.runtimes CASCADE;`, chainID))
	b.WriteString(fmt.Sprintf(`
INSERT INTO %s.runtimes (id, suspended, kind, tee_hardware, key_manager)
VALUES
`, chainID))
	for i, runtime := range document.Registry.Runtimes {
		keyManager := "none"
		if runtime.KeyManager != nil {
			keyManager = runtime.KeyManager.String()
		}
		b.WriteString(fmt.Sprintf(
			"\t('%s', %t, '%s', '%s', '%s')",
			runtime.ID.String(),
			false,
			runtime.Kind.String(),
			runtime.TEEHardware.String(),
			keyManager,

			// TODO(ennsharma): Add extra_data.
		))

		if i != len(document.Registry.Runtimes)-1 {
			b.WriteString(",\n")
		}
	}
	b.WriteString(";\n")

	b.WriteString(fmt.Sprintf(`
INSERT INTO %s.runtimes (id, suspended, kind, tee_hardware, key_manager)
VALUES
`, chainID))

	for i, runtime := range document.Registry.SuspendedRuntimes {
		keyManager := "none"
		if runtime.KeyManager != nil {
			keyManager = runtime.KeyManager.Hex()
		}
		b.WriteString(fmt.Sprintf(
			"\t('%s', %t, '%s', '%s', '%s')",
			runtime.ID.String(),
			true,
			runtime.Kind.String(),
			runtime.TEEHardware.String(),
			keyManager,

			// TODO(ennsharma): Add extra_data.
		))

		if i != len(document.Registry.SuspendedRuntimes)-1 {
			b.WriteString(",\n")
		}
	}
	b.WriteString(";\n")

	return nil
}

func (mg *MigrationGenerator) addStakingBackendMigrations(b *strings.Builder, document *genesis.Document) error {
	chainID := strcase.ToSnake(document.ChainID)

	// Populate accounts.
	b.WriteString(fmt.Sprintf(`
-- Staking Backend Data
TRUNCATE %s.accounts CASCADE;`, chainID))
	b.WriteString(fmt.Sprintf(`
INSERT INTO %s.accounts (address, general_balance, nonce, escrow_balance_active, escrow_total_shares_active, escrow_balance_debonding, escrow_total_shares_debonding)
VALUES
`, chainID))

	i := 0
	for address, account := range document.Staking.Ledger {
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

		if i != len(document.Staking.Ledger) {
			b.WriteString(",\n")
		}
	}
	b.WriteString(";\n")

	// Populate allowances.
	b.WriteString(fmt.Sprintf(`
TRUNCATE %s.allowances CASCADE;`, chainID))

	foundAllowances := false // in case allowances are empty

	i = 0
	for owner, account := range document.Staking.Ledger {
		if len(account.General.Allowances) > 0 && !foundAllowances {
			b.WriteString(fmt.Sprintf(`
INSERT INTO %s.allowances (owner, beneficiary, allowance)
VALUES
`, chainID))
			foundAllowances = true
		}

		ownerAllowances := make([]string, len(account.General.Allowances))
		j := 0
		for beneficiary, allowance := range account.General.Allowances {
			ownerAllowances[j] = fmt.Sprintf(
				"\t('%s', '%s', %d)",
				owner.String(),
				beneficiary.String(),
				allowance.ToBigInt(),
			)
			j++
		}
		b.WriteString(strings.Join(ownerAllowances, ",\n"))
		i++

		if i != len(document.Staking.Ledger) && len(account.General.Allowances) > 0 {
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
	for delegatee, escrows := range document.Staking.Delegations {
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

		if i != len(document.Staking.Delegations) && len(escrows) > 0 {
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
	for delegatee, escrows := range document.Staking.DebondingDelegations {
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

		if i != len(document.Staking.DebondingDelegations) && len(escrows) > 0 {
			b.WriteString(",\n")
		}
	}
	b.WriteString(";\n")

	return nil
}

func (mg *MigrationGenerator) addGovernanceBackendMigrations(b *strings.Builder, document *genesis.Document) error {
	chainID := strcase.ToSnake(document.ChainID)

	// Populate proposals.
	b.WriteString(fmt.Sprintf(`
-- Governance Backend Data
TRUNCATE %s.proposals CASCADE;`, chainID))

	if len(document.Governance.Proposals) > 0 {

		// TODO(ennsharma): Extract `executed` for proposal.
		b.WriteString(fmt.Sprintf(`
INSERT INTO %s.proposals (id, submitter, state, deposit, handler, cp_target_version, rhp_target_version, rcp_target_version, upgrade_epoch, cancels, created_at, closes_at, invalid_votes)
VALUES
`, chainID))

		for i, proposal := range document.Governance.Proposals {
			if proposal.Content.Upgrade != nil {
				b.WriteString(fmt.Sprintf(
					"\t(%d, '%s', '%s', %d, '%s', '%s', '%s', '%s', %d, %d, %d, %d, %d)",
					proposal.ID,
					proposal.Submitter.String(),
					proposal.State.String(),
					proposal.Deposit.ToBigInt(),
					proposal.Content.Upgrade.Handler,
					proposal.Content.Upgrade.Target.ConsensusProtocol.String(),
					proposal.Content.Upgrade.Target.RuntimeHostProtocol.String(),
					proposal.Content.Upgrade.Target.RuntimeCommitteeProtocol.String(),
					proposal.Content.Upgrade.Epoch,
					0,
					proposal.CreatedAt,
					proposal.ClosesAt,
					proposal.InvalidVotes,
				))
			} else if proposal.Content.CancelUpgrade != nil {
				b.WriteString(fmt.Sprintf(
					"\t(%d, '%s', '%s', %d, '%s', '%s', '%s', '%s', '%s', %d, %d, %d, %d)",
					proposal.ID,
					proposal.Submitter.String(),
					proposal.State.String(),
					proposal.Deposit.ToBigInt(),
					"",
					"",
					"",
					"",
					"",
					proposal.Content.CancelUpgrade.ProposalID,
					proposal.CreatedAt,
					proposal.ClosesAt,
					proposal.InvalidVotes,
				))
			}

			if i != len(document.Governance.Proposals)-1 {
				b.WriteString(",\n")
			}
		}
		b.WriteString(";\n")
	}

	// Populate votes.
	b.WriteString(fmt.Sprintf(`
TRUNCATE %s.votes CASCADE;`, chainID))

	foundVotes := false // in case votes are empty

	i := 0
	for proposalID, voteEntries := range document.Governance.VoteEntries {
		if len(voteEntries) > 0 && !foundVotes {
			b.WriteString(fmt.Sprintf(`
INSERT INTO %s.votes (proposal, voter, vote)
VALUES
`, chainID))
			foundVotes = true
		}
		votes := make([]string, len(voteEntries))
		for j, voteEntry := range voteEntries {
			votes[j] = fmt.Sprintf(
				"\t(%d, '%s', '%s')",
				proposalID,
				voteEntry.Voter.String(),
				voteEntry.Vote.String(),
			)
		}
		b.WriteString(strings.Join(votes, ",\n"))
		i++

		if i != len(document.Governance.VoteEntries) && len(voteEntries) > 0 {
			b.WriteString(",\n")
		}
	}
	if foundVotes {
		b.WriteString(";\n")
	}

	return nil
}
