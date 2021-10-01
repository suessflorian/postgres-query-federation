package main

import (
	"context"
	_ "embed"
	"fmt"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v4"
	log "github.com/sirupsen/logrus"
)

//go:embed migration.sql
var MIGRATION string

var TENANT_COUNT int = 80
var PER_TENANT_ENTITY_COUNT int = 1200

func init() {
	ctx := context.Background()

	conn, err := newConnection(ctx, databaseConfig{"localhost", "user", "password", "postgres"})
	if err != nil {
		panic(err)
	}
	defer conn.Close(ctx)

	// yucky way around lack of CREATE DATABASE IF NOT EXISTS
	var res string
	if err := conn.QueryRow(ctx, "SELECT datname FROM pg_database WHERE datname LIKE 'tenant_%'").Scan(&res); err == pgx.ErrNoRows {
		log.Info("creating tenant databases with mock data, may take a moment...")
		for _, tenant := range listOfTenants() {
			if _, err = conn.Exec(ctx, "CREATE DATABASE "+tenant); err != nil {
				log.WithError(err).Fatal("failed to create tenant databases")
			}

			tenantConn, err := newConnection(ctx, databaseConfig{"localhost", "user", "password", tenant})
			if err != nil {
				log.WithError(err).Fatal("can't connect to tenant")
			}

			if _, err := tenantConn.Exec(ctx, MIGRATION); err != nil {
				log.WithError(err).Fatal("can't create schema tenant")
			}

			if _, err := tenantConn.CopyFrom(ctx, pgx.Identifier{"entities"}, []string{"id", "last_updated"}, newMockGen(PER_TENANT_ENTITY_COUNT)); err != nil {
				log.WithError(err).Fatal("can't gen mock data")
			}

			tenantConn.Close(ctx)
		}
	} else if err != nil {
		panic(err)
	}
}

func main() {
	ctx := context.Background()
	log.Info("creating federated view")

	conn, err := newConnection(ctx, databaseConfig{"localhost", "user", "password", "postgres"})
	if err != nil {
		log.WithError(err).Fatal("can't connect to federation host db")
	}
	defer conn.Close(ctx)

	// enable usage of `postgres_fdw`
	if _, err := conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS postgres_fdw"); err != nil {
		log.WithError(err).Fatal("can't create foreign data server for db")
	}

	for _, tenant := range listOfTenants() {
		log := log.WithField("tenant", tenant)

		// 1. foreign data server (handles connection to foreign data source)
		if _, err := conn.Exec(ctx, fmt.Sprintf(`CREATE SERVER IF NOT EXISTS %s_foreign_data_wrapper
        FOREIGN DATA WRAPPER postgres_fdw
          OPTIONS (DBNAME '%s')
    `, tenant, tenant)); err != nil {
			log.WithError(err).Fatal("can't create foreign data server for db")
		}

		// 2. provide connection details for foreign data
		if _, err := conn.Exec(ctx, fmt.Sprintf(`CREATE USER MAPPING IF NOT EXISTS FOR %s SERVER %s_foreign_data_wrapper OPTIONS (user '%s')`, "user", tenant, "user")); err != nil {
			log.WithError(err).Fatal("can't add foreign data connection deets")
		}

		// 3. foreign table
		if _, err = conn.Exec(ctx, fmt.Sprintf(`
        CREATE FOREIGN TABLE IF NOT EXISTS %s_entities(
          id int,
          last_updated timestamp
        ) SERVER %s_foreign_data_wrapper OPTIONS( TABLE_NAME 'entities')`, tenant, tenant)); err != nil {
			log.WithError(err).Fatal("can't create foreign table")
		}
	}
}

type databaseConfig struct {
	host     string
	user     string
	password string
	dbname   string
}

func newConnection(ctx context.Context, cfg databaseConfig) (*pgx.Conn, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable", cfg.host, cfg.user, cfg.password, cfg.dbname)
	connConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DSN config %w", err)
	}

	return pgx.ConnectConfig(ctx, connConfig)
}

func listOfTenants() []string {
	var tenants []string
	for tenant := 0; tenant <= TENANT_COUNT; tenant++ {
		tenants = append(tenants, fmt.Sprintf("tenant_%d", tenant))
	}
	return tenants
}

func newMockGen(maxEntites int) *mock {
	return &mock{max: rand.Intn(maxEntites)}
}

type mock struct {
	i   int
	max int
}

func (s *mock) Next() bool { return s.i < s.max }
func (s *mock) Err() error { return nil }
func (s *mock) Values() ([]interface{}, error) {
	var res []interface{}
	res = append(res, s.i)

	// timestamp randomly within an hour ago
	withinHourAgo := rand.Int63n(int64(time.Hour))
	randStamp := time.Now().UnixNano() - int64(time.Hour) + withinHourAgo
	res = append(res, time.Unix(randStamp/int64(time.Second), 0))

	s.i++
	return res, nil
}
