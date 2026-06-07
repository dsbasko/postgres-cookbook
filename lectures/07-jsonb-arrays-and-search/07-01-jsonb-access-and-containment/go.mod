module github.com/dsbasko/postgres-cookbook/lectures/07-jsonb-arrays-and-search/07-01-jsonb-access-and-containment

go 1.26

require (
	github.com/dsbasko/postgres-cookbook/lectures/internal v0.0.0-00010101000000-000000000000
	github.com/jackc/pgx/v5 v5.9.2
)

// internal — workspace member, локально через replace (как и в go.work).
replace github.com/dsbasko/postgres-cookbook/lectures/internal => ../../internal

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)
