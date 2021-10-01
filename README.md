# Postgres query federation via foreign tables

Serves as an example of foreign data federation in Postgres as per [SQL/MED](https://wiki.postgresql.org/wiki/SQL/MED).

Postgres is shipped with the [`postgres_fdw`](https://www.postgresql.org/docs/13/postgres-fdw.html) module. Providing the ability to create [foreign data wrappers](https://www.postgresql.org/docs/13/sql-createforeigndatawrapper.html) that are used to establish [servers](https://www.postgresql.org/docs/13/sql-createserver.html) that populate locally queryable [foreign tables](https://www.postgresql.org/docs/13/sql-createforeigntable.html).

Consider a separate database to that of any of the tenants' databases;

```sql
CREATE SERVER tenant_a_fdw_target
  FOREIGN DATA WRAPPER postgres_fdw
    OPTIONS
      (DBNAME 'tenant_a', HOST 'our-rds-cluster.region.rds.amazonaws.com', SSLMODE 'require');

CREATE FOREIGN TABLE tenant_a_entities(
  id uuid,
  last_updated timestamp
) SERVER tenant_a_fdw_target OPTIONS( TABLE_NAME 'entities');
```

Creates a locally querable construct called `foreign table`.
```sql
-- \dE to list foreign tables

-- for example, staleness monitoring of entities
SELECT
  id,
  EXTRACT(EPOCH FROM (current_timestamp - min(last_updated))) AS time_since_refreshed
FROM ${tenant}_entities group;
```

## Use case

Simply put; cross data source queries, data source being anything queryable.

**This is not limited to postgres databases**, could be anything that has an associated Postgres foreign data wrapper. [ Exhaustive list here](https://wiki.postgresql.org/wiki/Foreign_data_wrappers).

_Note; there is currently no such wrapper for a GraphQL data source (as of Oct 1st 2021)_.

## Example here

[Grafana](https://github.com/grafana/grafana) is a very popular monitoring and observability platform that namely supports the ingestion of [an increasing list](https://grafana.com/docs/grafana/latest/datasources) of datasources. However, Grafana defines a datasources via a single connection. Problems:

- In a tenant per database architecture, the tenant count would linearly affect the quantity of data sources required for full monitoring coverage.
- This is especially a problem with regard to multi-service architectures (for example, [nano-services](https://github.com/movio/red) 😉 jk jk), as service boundaries are quite commonly define database boundaries. This suggests service count could also linearly affect the quantity of data sources required for full monitoring coverage.

Consequently creating quite a bit of management overhead:

- Grafana advices [read only](https://grafana.com/docs/grafana/latest/enterprise/datasource_permissions/) data source role permissions to help restrict unintended data exposure and/or accidental data damages.
- Grafana handling an increasing amount database connections, effecting [dashboard](https://grafana.com/grafana/dashboards) loading times significantly.
- Grafana alert management will also be forced per data source.
- Motivates the need for tenant implementation automation (extra work required for setting up a tenant).

### Solution

This setup here will simulate the utilisation of query federation to aggregate cross database metrics resembling that of a typical multi-tenanted architecture.

```sh
docker compose up
```
