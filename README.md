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
│   ├── middleware/  # Middleware (JWT, CORS)
│   └── repositories/ # Repositórios de dados
├── migrations/      # Migrations do banco de dados
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
```

2. Instale a ferramenta de migrations (golang-migrate):

```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

3. Execute as migrations para criar o banco de dados:

```bash
# Subir migrations (criar tabelas)
migrate -path migrations -database "postgres://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=disable" up

# Desfazer última migration
migrate -path migrations -database "postgres://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=disable" down

# Verificar versão atual
migrate -path migrations -database "postgres://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=disable" version
```

## Criando Novas Migrations

Para criar uma nova migration:

```bash
# Criar nova migration (substitua NOME pela descrição)
migrate create -ext sql -dir migrations -seq NOME_DA_MIGRATION
```

Isso criará dois arquivos:
- `NNNNNNN_NOME_DA_MIGRATION.up.sql` - SQL para aplicar a mudança
- `NNNNNNN_NOME_DA_MIGRATION.down.sql` - SQL para reverter a mudança

Exemplo:
```bash
migrate create -ext sql -dir migrations -seq add_user_avatar
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
