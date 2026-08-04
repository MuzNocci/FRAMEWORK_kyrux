package database

// Kyrux não importa nenhum driver — a responsabilidade é do desenvolvedor.
// Adicione o driver desejado no go.mod e importe com blank identifier no seu main.go ou apps.
//
// Exemplo:
//
//	import _ "github.com/lib/pq"
//
// Em seguida, registre a conexão no bootstrap ou na inicialização do seu app:
//
//	fw.DB.Add("default", "postgres", environment.Get("DB_DSN"))
//	fw.DB.Add("analytics", "postgres", environment.Get("ANALYTICS_DSN"))
//
// Use a conexão:
//
//	db := fw.DB.Use()            // conexão "default"
//	db := fw.DB.Use("analytics") // conexão nomeada
//
// ─────────────────────────────────────────────────────────────────
// DRIVERS SQL SUPORTADOS
// ─────────────────────────────────────────────────────────────────
//
// PostgreSQL
//   driver: "postgres"
//   módulo: github.com/lib/pq
//   dsn:    postgres://user:password@host:5432/dbname?sslmode=disable
//
// PostgreSQL (pgx)
//   driver: "pgx"
//   módulo: github.com/jackc/pgx/v5/stdlib
//   dsn:    postgres://user:password@host:5432/dbname?sslmode=disable
//   nota:   mais ativo/moderno que lib/pq, mas NÃO é mais rápido aqui —
//           medido ~15-25% mais lento que lib/pq num SELECT de 20 linhas
//           via database/sql (a vantagem de protocolo binário do pgx só
//           aparece na API nativa dele, fora de database/sql — que é como
//           o ORM sempre usa). Prefira lib/pq a menos que precise de algum
//           recurso específico do pgx.
//
// MySQL / MariaDB
//   driver: "mysql"
//   módulo: github.com/go-sql-driver/mysql
//   dsn:    user:password@tcp(host:3306)/dbname?charset=utf8mb4&parseTime=True
//
// SQLite (sem CGO)
//   driver: "sqlite"
//   módulo: modernc.org/sqlite
//   dsn:    ./data.db
//
// SQLite (com CGO)
//   driver: "sqlite3"
//   módulo: github.com/mattn/go-sqlite3
//   dsn:    ./data.db
//
// SQL Server
//   driver: "sqlserver"
//   módulo: github.com/microsoft/go-mssqldb
//   dsn:    sqlserver://user:password@host:1433?database=dbname
//
// Oracle (sem CGO — puro Go)
//   driver: "oracle"
//   módulo: github.com/sijms/go-ora/v2
//   dsn:    oracle://user:password@host:1521/service
//
// ─────────────────────────────────────────────────────────────────
// DRIVERS NoSQL / OUTROS (via client nativo, não database/sql)
// ─────────────────────────────────────────────────────────────────
//
// MongoDB
//   módulo: go.mongodb.org/mongo-driver/mongo
//   uso:    client nativo, não usa database/sql nem este Manager
//
// Redis
//   módulo: github.com/redis/go-redis/v9
//   uso:    client nativo via core/cache ou direto
//
// Cassandra
//   módulo: github.com/gocql/gocql
//   uso:    client nativo, não usa database/sql
//
// DynamoDB
//   módulo: github.com/aws/aws-sdk-go-v2/service/dynamodb
//   uso:    SDK AWS, não usa database/sql
