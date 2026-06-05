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
├── main.go          # Entry point
├── database.sql     # Schema do banco de dados
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

2. Execute o script SQL para criar o banco de dados:

```bash
psql -U postgres -d bolao_copa -f database.sql
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
