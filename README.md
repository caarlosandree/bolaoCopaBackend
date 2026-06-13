# Bolão da Copa - Backend

Backend em Go para o sistema de bolão da Copa do Mundo Corporativo.

## Tecnologias

- **Go 1.26**
- **Echo Framework** - Framework web
- **PostgreSQL** - Banco de dados
- **JWT** - Autenticação
- **pgx** - Driver PostgreSQL

## Estrutura do Projeto

```
backend/
├── internal/
│   ├── config/      # Configurações
│   ├── db/          # Conexão com banco de dados
│   ├── handlers/    # Handlers HTTP
│   ├── migrations/  # Runner e SQLs de migrations embutidas
│   ├── middleware/  # Middleware (JWT, CORS)
│   └── repositories/ # Repositórios de dados
├── main.go          # Entry point
├── database.sql     # Schema do banco de dados (legado)
└── go.mod           # Dependências Go
```

## Configuração

1. Crie um arquivo `.env` na raiz do projeto:

```env
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=bolao_copa
JWT_SECRET=your_jwt_secret
MATCH_SYNC_ENABLED=true
MATCH_RESULT_CHECK_AFTER_MINUTES=120
MATCH_RESULT_RETRY_MINUTES=15
THESPORTSDB_API_KEY=your_api_key
```

O sync de calendário e resultados usa exclusivamente a API **TheSportsDB** (fonte da
verdade). A cada `MATCH_RESULT_RETRY_MINUTES`, o backend busca o schedule atualizado e
atualiza placares e status (`scheduled` / `ongoing` / `finished`). Quando uma partida
aparece como finalizada (`FT`), o placar é gravado e os pontos dos palpites são
recalculados automaticamente.

2. Execute o servidor. As migrations em `internal/migrations/sql/*.up.sql` são
   aplicadas automaticamente no startup:

```bash
go run main.go
```

## Criando Novas Migrations

Crie um arquivo `.up.sql` em `internal/migrations/sql/` seguindo a sequência
numérica usada pelo runner:

```bash
touch internal/migrations/sql/000004_nome_da_migration.up.sql
```

## Instalação

```bash
# Instalar dependências
go mod download

# Executar o servidor
go run main.go
```

O servidor rodará em `http://localhost:1323`

## Endpoints

### Público
- `POST /api/auth/register` - Registro de usuário
- `POST /api/auth/login` - Login
- `GET /api/ranking` - Ranking de usuários
- `GET /health` - Health check

### Autenticado (JWT)
- `GET /api/rounds/active` - Rodada ativa
- `POST /api/guesses` - Salvar palpite

### Admin
- `POST /api/admin/matches/:id/score` - Atualizar placar da partida

## Desenvolvimento

```bash
# Formatar código
go fmt ./...

# Rodar testes
go test ./...

# Build
go build -o bolao-backend
```
