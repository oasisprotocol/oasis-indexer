package genesis

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"testing"

	"github.com/iancoleman/strcase"
	"github.com/oasisprotocol/oasis-core/go/common/entity"
	governance "github.com/oasisprotocol/oasis-core/go/governance/api"
	registry "github.com/oasisprotocol/oasis-core/go/registry/api"
	staking "github.com/oasisprotocol/oasis-core/go/staking/api"
	oasisConfig "github.com/oasisprotocol/oasis-sdk/client-sdk/go/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oasislabs/oasis-indexer/log"
	"github.com/oasislabs/oasis-indexer/storage"
	"github.com/oasislabs/oasis-indexer/storage/oasis"
	"github.com/oasislabs/oasis-indexer/storage/postgres"
	"github.com/oasislabs/oasis-indexer/tests"
)

type TestEntity struct {
	ID    string
	Nodes []string
}

type TestNode struct {
	ID              string
	EntityID        string
	Expiration      uint64
	TlsPubkey       string
	TlsNextPubkey   string
	P2pPubkey       string
	ConsensusPubkey string
	VrfPubkey       string
	Roles           string
	SoftwareVersion string
}

type TestRuntime struct {
	ID          string
	Suspended   bool
	Kind        string
	TeeHardware string
	KeyManager  string
}

type TestAccount struct {
	Address   string
	Nonce     uint64
	Available uint64
	Escrow    uint64
	Debonding uint64
}

type TestProposal struct {
	ID               uint64
	Submitter        string
	State            string
	Executed         bool
	Deposit          uint64
	Handler          *string
	CpTargetVersion  *string
	RhpTargetVersion *string
	RcpTargetVersion *string
	UpgradeEpoch     *uint64
	Cancels          *uint64
	CreatedAt        uint64
	ClosesAt         uint64
	InvalidVotes     uint64
}

type TestVote struct {
	Proposal uint64
	Voter    string
	Vote     string
}

func newTargetClient(t *testing.T) (*postgres.Client, error) {
	connString := os.Getenv("CI_TEST_CONN_STRING")
	logger, err := log.NewLogger("cockroach-test", ioutil.Discard, log.FmtJSON, log.LevelInfo)
	assert.Nil(t, err)

	return postgres.NewClient(connString, logger)
}

func newSourceClient(t *testing.T) (*oasis.Client, error) {
	network := &oasisConfig.Network{
		ChainContext: os.Getenv("CI_TEST_CHAIN_CONTEXT"),
		RPC:          os.Getenv("CI_TEST_NODE_RPC"),
	}
	return oasis.NewClient(context.Background(), network)
}

func getChainID(ctx context.Context, t *testing.T, source *oasis.Client) string {
	doc, err := source.GenesisDocument(ctx)
	assert.Nil(t, err)
	return strcase.ToSnake(doc.ChainID)
}

func checkpointBackends(t *testing.T, source *oasis.Client, target *postgres.Client) (int64, error) {
	ctx := context.Background()

	chainID := getChainID(ctx, t, source)

	// Create checkpoint tables.
	batch := &storage.QueryBatch{}
	for _, t := range []string{
		// Registry backend.
		"entities",
		"nodes",
		"runtimes",
		// Staking backend.
		"accounts",
		"allowances",
		"delegations",
		"debonding_delegations",
		// Governance backend.
		"proposals",
		"votes",
	} {
		batch.Queue(fmt.Sprintf(`
			DROP TABLE IF EXISTS %s.%s_checkpoint CASCADE;
		`, chainID, t))
		batch.Queue(fmt.Sprintf(`
			CREATE TABLE %s.%s_checkpoint AS TABLE %s.%s;
		`, chainID, t, chainID, t))
	}
	batch.Queue(fmt.Sprintf(`
			INSERT INTO %s.checkpointed_heights (height)
				SELECT height FROM %s.processed_blocks ORDER BY height DESC, processed_time DESC LIMIT 1
				ON CONFLICT DO NOTHING;
		`, chainID, chainID))

	if err := target.SendBatch(ctx, batch); err != nil {
		return 0, err
	}

	var checkpointHeight int64
	if err := target.QueryRow(ctx, fmt.Sprintf(`
		SELECT height FROM %s.checkpointed_heights
			ORDER BY height DESC LIMIT 1;
	`, chainID)).Scan(&checkpointHeight); err != nil {
		return 0, err
	}

	return checkpointHeight, nil
}

func TestBlocksSanityCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping testing in short mode")
	}
	if _, ok := os.LookupEnv("OASIS_INDEXER_HEALTHCHECK"); !ok {
		t.Skip("skipping test since healthcheck tests are not enabled")
	}

	ctx := context.Background()

	oasisClient, err := newSourceClient(t)
	require.Nil(t, err)

	postgresClient, err := newTargetClient(t)
	require.Nil(t, err)

	doc, err := oasisClient.GenesisDocument(ctx)
	require.Nil(t, err)
	chainID := strcase.ToSnake(doc.ChainID)

	var latestHeight int64
	err = postgresClient.QueryRow(ctx, fmt.Sprintf(
		`SELECT height FROM %s.blocks ORDER BY height DESC LIMIT 1;`,
		chainID,
	)).Scan(&latestHeight)
	require.Nil(t, err)

	var actualHeightSum int64
	err = postgresClient.QueryRow(ctx, fmt.Sprintf(
		`SELECT SUM(height) FROM %s.blocks WHERE height <= $1;`,
		chainID,
	), latestHeight).Scan(&actualHeightSum)
	require.Nil(t, err)

	// Using formula for sum of first k natural numbers.
	expectedHeightSum := latestHeight*(latestHeight+1)/2 - (tests.GenesisHeight-1)*tests.GenesisHeight/2
	require.Equal(t, expectedHeightSum, actualHeightSum)
}

func TestGenesisFull(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping testing in short mode")
	}
	if _, ok := os.LookupEnv("OASIS_INDEXER_HEALTHCHECK"); !ok {
		t.Skip("skipping test since healthcheck tests are not enabled")
	}

	t.Log("Initializing data stores...")

	ctx := context.Background()

	oasisClient, err := newSourceClient(t)
	assert.Nil(t, err)

	postgresClient, err := newTargetClient(t)
	assert.Nil(t, err)

	t.Log("Creating checkpoint...")

	height, err := checkpointBackends(t, oasisClient, postgresClient)
	assert.Nil(t, err)

	t.Logf("Validating at height %d...", height)

	registryGenesis, err := oasisClient.RegistryGenesis(ctx, height)
	require.Nil(t, err)
	validateRegistryBackend(t, registryGenesis, oasisClient, postgresClient)

	// stakingGenesis, err := oasisClient.StakingGenesis(ctx, height)
	// require.Nil(t, err)
	// validateStakingBackend(t, stakingGenesis, oasisClient, postgresClient)

	governanceGenesis, err := oasisClient.GovernanceGenesis(ctx, height)
	require.Nil(t, err)
	validateGovernanceBackend(t, governanceGenesis, oasisClient, postgresClient)
}

func validateRegistryBackend(t *testing.T, genesis *registry.Genesis, source *oasis.Client, target *postgres.Client) {
	ctx := context.Background()

	chainID := getChainID(ctx, t, source)

	t.Log("Validating registry backend...")

	// WHY: There's a number of nodes that are not registered via `RegisterNode` in the official genesis state.
	expectedEntities := make(map[string]TestEntity)
	for _, se := range genesis.Entities {
		if se == nil {
			continue
		}
		var e entity.Entity
		err := se.Open(registry.RegisterEntitySignatureContext, &e)
		assert.Nil(t, err)

		te := TestEntity{
			ID:    e.ID.String(),
			Nodes: make([]string, len(e.Nodes)),
		}
		for i, n := range e.Nodes {
			te.Nodes[i] = n.String()
		}
		sort.Slice(te.Nodes, func(i, j int) bool {
			return te.Nodes[i] < te.Nodes[j]
		})

		expectedEntities[te.ID] = te
	}

	entityRows, err := target.Query(ctx, fmt.Sprintf(
		`SELECT id FROM %s.entities_checkpoint`, chainID),
	)
	require.Nil(t, err)

	actualEntities := make(map[string]TestEntity)
	for entityRows.Next() {
		var e TestEntity
		err = entityRows.Scan(
			&e.ID,
		)
		assert.Nil(t, err)

		nodeRows, err := target.Query(ctx, fmt.Sprintf(
			`SELECT id FROM %s.nodes_checkpoint WHERE entity_id = $1`, chainID),
			e.ID)
		assert.Nil(t, err)

		e.Nodes = make([]string, 0)
		for nodeRows.Next() {
			var nid string
			err = nodeRows.Scan(
				&nid,
			)
			assert.Nil(t, err)
			e.Nodes = append(e.Nodes, nid)
		}
		sort.Slice(e.Nodes, func(i, j int) bool {
			return e.Nodes[i] < e.Nodes[j]
		})

		actualEntities[e.ID] = e
	}

	assert.Equal(t, len(expectedEntities), len(actualEntities))
	for ke, ve := range expectedEntities {
		va, ok := actualEntities[ke]
		if !ok {
			t.Logf("entity %s expected, but not found", ke)
			continue
		}
		assert.Equal(t, ve, va)
	}

	// expectedNodes := make(map[string]TestNode)
	// for _, sn := range genesis.Nodes {
	// 	if sn == nil {
	// 		continue
	// 	}
	// 	var n node.Node
	// 	err := sn.Open(registry.RegisterNodeSignatureContext, &n)
	// 	assert.Nil(t, err)

	// 	vrfPubkey := ""
	// 	if n.VRF != nil {
	// 		vrfPubkey = n.VRF.ID.String()
	// 	}
	// 	tn := TestNode{
	// 		ID:              n.ID.String(),
	// 		EntityID:        n.EntityID.String(),
	// 		Expiration:      n.Expiration,
	// 		TlsPubkey:       n.TLS.PubKey.String(),
	// 		TlsNextPubkey:   n.TLS.NextPubKey.String(),
	// 		P2pPubkey:       n.P2P.ID.String(),
	// 		VrfPubkey:       vrfPubkey,
	// 		Roles:           n.Roles.String(),
	// 		SoftwareVersion: n.SoftwareVersion,
	// 	}

	// 	expectedNodes[tn.ID] = tn
	// }

	// nodeRows, err := target.Query(ctx, fmt.Sprintf(
	// 	`SELECT
	// 		id, entity_id, expiration,
	// 		tls_pubkey, tls_next_pubkey, p2p_pubkey,
	// 		vrf_pubkey, roles, software_version
	// 	FROM %s.nodes_checkpoint`, chainID),
	// )
	// require.Nil(t, err)

	// actualNodes := make(map[string]TestNode)
	// for nodeRows.Next() {
	// 	var n TestNode
	// 	err = nodeRows.Scan(
	// 		&n.ID,
	// 		&n.EntityID,
	// 		&n.Expiration,
	// 		&n.TlsPubkey,
	// 		&n.TlsNextPubkey,
	// 		&n.P2pPubkey,
	// 		&n.VrfPubkey,
	// 		&n.Roles,
	// 		&n.SoftwareVersion,
	// 	)
	// 	assert.Nil(t, err)

	// 	actualNodes[n.ID] = n
	// }

	// assert.Equal(t, len(expectedNodes), len(actualNodes))
	// for ke, ve := range expectedNodes {
	// 	va, ok := actualNodes[ke]
	// 	if !ok {
	// 		t.Logf("node %s expected, but not found", ke)
	// 		continue
	// 	}
	// 	assert.Equal(t, ve, va)
	// }
	// for ka, va := range actualNodes {
	// 	ve, ok := expectedNodes[ka]
	// 	if !ok {
	// 		t.Logf("node %s found, but not expected", ka)
	// 		continue
	// 	}
	// 	assert.Equal(t, ve, va)
	// }

	expectedRuntimes := make(map[string]TestRuntime)
	for _, r := range genesis.Runtimes {
		if r == nil {
			continue
		}

		keyManager := "none"
		if r.KeyManager != nil {
			keyManager = r.KeyManager.String()
		}
		tr := TestRuntime{
			ID:          r.ID.String(),
			Suspended:   false,
			Kind:        r.Kind.String(),
			TeeHardware: r.TEEHardware.String(),
			KeyManager:  keyManager,
		}

		expectedRuntimes[tr.ID] = tr
	}
	for _, r := range genesis.SuspendedRuntimes {
		if r == nil {
			continue
		}

		keyManager := "none"
		if r.KeyManager != nil {
			keyManager = r.KeyManager.String()
		}
		tr := TestRuntime{
			ID:          r.ID.String(),
			Suspended:   true,
			Kind:        r.Kind.String(),
			TeeHardware: r.TEEHardware.String(),
			KeyManager:  keyManager,
		}

		expectedRuntimes[tr.ID] = tr
	}

	runtimeRows, err := target.Query(ctx, fmt.Sprintf(
		`SELECT id, suspended, kind, tee_hardware, key_manager FROM %s.runtimes_checkpoint`, chainID),
	)
	require.Nil(t, err)

	actualRuntimes := make(map[string]TestRuntime)
	for runtimeRows.Next() {
		var tr TestRuntime
		err = runtimeRows.Scan(
			&tr.ID,
			&tr.Suspended,
			&tr.Kind,
			&tr.TeeHardware,
			&tr.KeyManager,
		)
		assert.Nil(t, err)

		actualRuntimes[tr.ID] = tr
	}

	assert.Equal(t, len(expectedRuntimes), len(actualRuntimes))
	for ke, ve := range expectedRuntimes {
		va, ok := actualRuntimes[ke]
		if !ok {
			t.Logf("runtime %s expected, but not found", ke)
			continue
		}
		assert.Equal(t, ve, va)
	}

	t.Log("Done validating registry backend!")
}

func validateStakingBackend(t *testing.T, genesis *staking.Genesis, source *oasis.Client, target *postgres.Client) {
	ctx := context.Background()

	chainID := getChainID(ctx, t, source)

	t.Log("Validating staking backend...")

	rows, err := target.Query(ctx, fmt.Sprintf(
		`SELECT address, nonce, general_balance
				FROM %s.accounts_checkpoint`, chainID),
	)
	require.Nil(t, err)
	for rows.Next() {
		var a TestAccount
		err = rows.Scan(
			&a.Address,
			&a.Nonce,
			&a.Available,
			// &a.Escrow,
			// &a.Debonding,
		)
		assert.Nil(t, err)

		var address staking.Address
		err = address.UnmarshalText([]byte(a.Address))
		assert.Nil(t, err)

		acct, ok := genesis.Ledger[address]
		if !ok {
			t.Logf("address %s expected, but not found", address.String())
			continue
		}

		e := TestAccount{
			Address:   address.String(),
			Nonce:     acct.General.Nonce,
			Available: acct.General.Balance.ToBigInt().Uint64(),
			// Escrow:    acct.Escrow.Active.Balance.ToBigInt().Uint64(),
			// Debonding: acct.Escrow.Debonding.Balance.ToBigInt().Uint64(),
		}
		assert.Equal(t, e, a)

	}

	t.Log("Done validating staking backend!")
}

func validateGovernanceBackend(t *testing.T, genesis *governance.Genesis, source *oasis.Client, target *postgres.Client) {
	ctx := context.Background()

	chainID := getChainID(ctx, t, source)

	t.Log("Validating governance backend...")

	expectedProposals := make(map[uint64]TestProposal)
	for _, p := range genesis.Proposals {
		if p == nil {
			continue
		}
		var ep TestProposal
		ep.ID = p.ID
		ep.Submitter = p.Submitter.String()
		ep.State = p.State.String()
		ep.Deposit = p.Deposit.ToBigInt().Uint64()
		if p.Content.Upgrade != nil {
			handler := string(p.Content.Upgrade.Handler)
			cpTargetVersion := p.Content.Upgrade.Target.ConsensusProtocol.String()
			rhpTargetVersion := p.Content.Upgrade.Target.RuntimeHostProtocol.String()
			rcpTargetVersion := p.Content.Upgrade.Target.RuntimeCommitteeProtocol.String()
			upgradeEpoch := uint64(p.Content.Upgrade.Epoch)

			ep.Handler = &handler
			ep.CpTargetVersion = &cpTargetVersion
			ep.RhpTargetVersion = &rhpTargetVersion
			ep.RcpTargetVersion = &rcpTargetVersion
			ep.UpgradeEpoch = &upgradeEpoch
		} else if p.Content.CancelUpgrade != nil {
			cancels := p.Content.CancelUpgrade.ProposalID
			ep.Cancels = &cancels
		} else {
			t.Logf("Malformed proposal %d", p.ID)
		}
		ep.CreatedAt = uint64(p.CreatedAt)
		ep.ClosesAt = uint64(p.ClosesAt)
		ep.InvalidVotes = p.InvalidVotes

		expectedProposals[ep.ID] = ep
	}

	proposalRows, err := target.Query(ctx, fmt.Sprintf(
		`SELECT id, submitter, state, executed, deposit,
						handler, cp_target_version, rhp_target_version, rcp_target_version, upgrade_epoch, cancels,
						created_at, closes_at, invalid_votes
				FROM %s.proposals_checkpoint`, chainID),
	)
	require.Nil(t, err)

	actualProposals := make(map[uint64]TestProposal)
	for proposalRows.Next() {
		var p TestProposal
		err = proposalRows.Scan(
			&p.ID,
			&p.Submitter,
			&p.State,
			&p.Executed,
			&p.Deposit,
			&p.Handler,
			&p.CpTargetVersion,
			&p.RhpTargetVersion,
			&p.RcpTargetVersion,
			&p.UpgradeEpoch,
			&p.Cancels,
			&p.CreatedAt,
			&p.ClosesAt,
			&p.InvalidVotes,
		)
		assert.Nil(t, err)
		actualProposals[p.ID] = p
	}

	assert.Equal(t, len(expectedProposals), len(actualProposals))
	for ke, ve := range expectedProposals {
		va, ok := actualProposals[ke]
		if !ok {
			t.Logf("proposal %d expected, but not found", ke)
			continue
		}
		assert.Equal(t, ve, va)
	}

	makeProposalKey := func(v TestVote) string {
		return fmt.Sprintf("%d.%s.%s", v.Proposal, v.Voter, v.Vote)
	}

	expectedVotes := make(map[string]TestVote)
	for p, ves := range genesis.VoteEntries {
		for _, ve := range ves {
			v := TestVote{
				Proposal: p,
				Voter:    ve.Voter.String(),
				Vote:     ve.Vote.String(),
			}
			expectedVotes[makeProposalKey(v)] = v
		}
	}

	voteRows, err := target.Query(ctx, fmt.Sprintf(
		`SELECT proposal, voter, vote
				FROM %s.votes_checkpoint`, chainID),
	)
	require.Nil(t, err)

	actualVotes := make(map[string]TestVote)
	for voteRows.Next() {
		var v TestVote
		err = voteRows.Scan(
			&v.Proposal,
			&v.Voter,
			&v.Vote,
		)
		assert.Nil(t, err)
		actualVotes[makeProposalKey(v)] = v
	}

	assert.Equal(t, len(expectedVotes), len(actualVotes))
	for ke, ve := range expectedVotes {
		va, ok := actualVotes[ke]
		if !ok {
			t.Logf("vote %s expected, but not found", ke)
			continue
		}
		assert.Equal(t, ve, va)
	}

	t.Log("Done validating governance backend!")
}
