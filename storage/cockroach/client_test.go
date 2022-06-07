package cockroach

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oasislabs/oasis-indexer/log"
)

func makeClient(t *testing.T) *Client {
	connString := os.Getenv("CI_TEST_CONN_STRING")
	logger := log.NewDefaultLogger("oasis-indexer-test")

	client, err := NewClient(connString, logger)
	require.Nil(t, err)

	return client
}

// func TestQuery(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip("skipping testing in short mode")
// 	}

// 	client := makeClient(t)
// 	defer client.Shutdown()

// 	rows, err := client.Query(context.Background(), `
// 		SELECT * FROM ( VALUES (0),(1),(2) ) AS q;
// 	`)
// 	require.Nil(t, err)

// 	i := 0
// 	for rows.Next() {
// 		var result int
// 		err = rows.Scan(&result)
// 		require.Nil(t, err)
// 		require.Equal(t, i, result)

// 		i++
// 	}
// 	require.Equal(t, 3, i)
// }

// func TestQueryRow(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip("skipping testing in short mode")
// 	}

// 	client := makeClient(t)
// 	defer client.Shutdown()

// 	row, err := client.QueryRow(context.Background(), `
// 		SELECT 1+1;
// 	`)
// 	require.Nil(t, err)

// 	var result int
// 	err = row.Scan(&result)
// 	require.Nil(t, err)
// 	require.Equal(t, 2, result)
// }

// func TestSendBatch(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip("skipping testing in short mode")
// 	}

// 	client := makeClient(t)
// 	defer client.Shutdown()

// 	defer func() {
// 		destroy := &storage.QueryBatch{}
// 		destroy.Queue(`
// 			DROP TABLE films;
// 		`)
// 		err := client.SendBatch(context.Background(), destroy)
// 		require.Nil(t, err)
// 	}()

// 	create := &storage.QueryBatch{}
// 	create.Queue(`
// 		CREATE TABLE films (
// 			fid  INTEGER PRIMARY KEY,
// 			name TEXT
// 		);
// 	`)
// 	err := client.SendBatch(context.Background(), create)
// 	require.Nil(t, err)

// 	insert := &storage.QueryBatch{}
// 	queueFilms := func(b *storage.QueryBatch, f []string, idOffset int) {
// 		rows := make([]string, 0, len(f))
// 		for i, film := range f {
// 			rows = append(rows, fmt.Sprintf("(%d, '%s')", i+idOffset, film))
// 		}
// 		b.Queue(fmt.Sprintf(`
// 			INSERT INTO films (fid, name)
// 			VALUES %s;
// 		`, strings.Join(rows, ", ")))
// 	}

// 	films1 := []string{
// 		"Gone with the Wind",
// 		"Avatar",
// 		"Titanic",
// 	}
// 	films2 := []string{
// 		"Star Wars",
// 		"Avengers: Endgame",
// 	}
// 	queueFilms(insert, films1, 0)
// 	queueFilms(insert, films2, len(films1))
// 	err = client.SendBatch(context.Background(), insert)
// 	require.Nil(t, err)

// 	var wg sync.WaitGroup
// 	for i, film := range append(films1, films2...) {
// 		wg.Add(1)
// 		go func(i int, film string) {
// 			defer wg.Done()

// 			var row storage.QueryResult
// 			row, err = client.QueryRow(context.Background(), `
// 				SELECT name FROM films WHERE fid = $1;
// 			`, i)
// 			var result string
// 			err = row.Scan(&result)
// 			require.Nil(t, err)
// 			require.Equal(t, film, result)
// 		}(i, film)
// 	}

// 	wg.Wait()
// }