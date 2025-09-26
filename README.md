# Python e Go no AWS Lambda: custo e desempenho com DynamoDB

Comparação aplicada entre **Python (interpretada)** e **Go (compilada)** em **AWS Lambda**, sob **SQS → Lambda → DynamoDB**, medindo **tempo** e **custo**. O objetivo é fornecer um protocolo **replicável**, 100% **Console AWS**, para levantar métricas e estimar custos por execução.

---

## 📌 Objetivo geral

Avaliar, em condições controladas, **tempo de execução** e **custo** de funções AWS Lambda escritas em Python e Go para uma carga **I/O‑bound** de inserções no **Amazon DynamoDB**, usando **Amazon SQS** como gatilho (**batch=1**) e **CloudWatch Logs** para coleta das métricas.

---

## 🧱 Arquitetura (visão rápida)

![Arquitetura infraestrutura e fluxo](./diagram/architecture_infrastructure.png)

**Região recomendada p/ TCC:** `us-east-1`  
**Memória:** `128 MB` (igual para ambas)  
**Concurrency:** `Reserved concurrency = 1` (evita “colds” adicionais)  
**SQS Batch size:** `1`

> **Nota**: Use a **mesma arquitetura** (ex.: `arm64`) em ambas as funções. Em Graviton/ARM, o custo de compute tende a ser ~**20% menor**.

---

## ✅ Pré‑requisitos

- Conta AWS com permissão para criar: **IAM Role, Lambda, SQS, DynamoDB, CloudWatch Logs**
- Sistema local para **compilar Go** (Windows/PowerShell ok)

---

## 1) Criar as tabelas DynamoDB (Console)

Crie **duas tabelas** (modo **On‑Demand**):

- `tcc_lambda_python`
- `tcc_lambda_go`

**Chaves (iguais nas duas):**  
`Partition key`: `pk` (String)  
`Sort key`: `sk` (String)

> Vamos gravar com `pk = "batch_id#<id>"` e `sk = "seq#<n>"` para facilitar a **idempotência**.

---

## 2) Criar as filas SQS (Console)

Crie **duas filas** (padrão):

- `tcc-python-queue`
- `tcc-go-queue`

**Configuração:**

- **Visibility timeout ≥** timeout da função Lambda (ex.: 2–5 min)
- Demais opções **padrão**

> _(Opcional)_ Você pode usar **uma fila única** e filtrar pelo **atributo** `lang`, mas **duas filas** deixam o ensaio mais simples.

---

## 3) Criar a Role do Lambda (Console → IAM)

**Nome:** `tcc-lambda-dynamo-role`  
**Trust (padrão para Lambda):** serviço confiável `lambda.amazonaws.com`

**Política** (anexar à role) — _ajuste os ARNs para sua conta/região se quiser restringir_:

```json
{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Effect": "Allow",
			"Action": [
				"logs:CreateLogGroup",
				"logs:CreateLogStream",
				"logs:PutLogEvents"
			],
			"Resource": "*"
		},
		{
			"Effect": "Allow",
			"Action": ["dynamodb:PutItem"],
			"Resource": [
				"arn:aws:dynamodb:us-east-1:YOUR_ACCOUNT_ID:table/tcc_lambda_python",
				"arn:aws:dynamodb:us-east-1:YOUR_ACCOUNT_ID:table/tcc_lambda_go"
			]
		},
		{
			"Effect": "Allow",
			"Action": [
				"sqs:ReceiveMessage",
				"sqs:DeleteMessage",
				"sqs:GetQueueAttributes"
			],
			"Resource": [
				"arn:aws:sqs:us-east-1:YOUR_ACCOUNT_ID:tcc-python-queue",
				"arn:aws:sqs:us-east-1:YOUR_ACCOUNT_ID:tcc-go-queue"
			]
		}
	]
}
```

---

## 4) Criar a Lambda **Python** (Console)

- **Function name:** `tcc-python`
- **Runtime:** `Python 3.12` _(ou 3.11 — ver nota abaixo)_
- **Architecture:** `arm64` _(use a mesma em Go)_
- **Role:** `tcc-lambda-dynamo-role`
- **Handler:** `lambda_function.handler`
- **Env var:** `TABLE_NAME = tcc_lambda_python`
- **Memory:** `128 MB`
- **Timeout:** `2–5 min`
- **Reserved concurrency:** `1`

> **⚠️ Importante (Python 3.12):** o runtime **3.12 não inclui o boto3** por padrão. Inclua o `boto3` no pacote de implantação (ou em uma camada). Se quiser evitar empacotar o SDK, use **Python 3.11**, que inclui o boto3 no runtime gerenciado.

**Código mínimo esperado** (`lambda_function.py`):

**Trigger SQS:** adicione a `tcc-python-queue` (**Batch size 1**).

---

## 5) Criar a Lambda **Go** (Console)

- **Function name:** `tcc-go`
- **Runtime:** `provided.al2` (custom)
- **Handler:** `bootstrap`
- **Architecture:** `arm64` _(igual à Python)_
- **Role:** `tcc-lambda-dynamo-role`
- **Env var:** `TABLE_NAME = tcc_lambda_go`
- **Memory:** `128 MB`
- **Timeout:** `2–5 min`
- **Reserved concurrency:** `1`

### Código mínimo sugerido (`/src/go/main.go`)

### Build (Windows / PowerShell)

```powershell
go mod tidy
$env:GOOS="linux"
$env:GOARCH="arm64"      # ou amd64, se preferir
$env:CGO_ENABLED="0"
go build -trimpath -ldflags="-s -w" -o bootstrap
Compress-Archive -Path .bootstrap -DestinationPath function.zip -Force
```

**Upload:** na Lambda → _Code_ → **Upload from .zip** → envie `function.zip`.  
**Trigger SQS:** adicione a `tcc-go-queue` (**Batch size 1**).

> **Pegadinhas:** Se aparecer _Handler not found_, confira `Handler = bootstrap` e o ZIP contendo `bootstrap` na **raiz**. Se der **“exec format error”**, a **arquitetura** da função não bate com `GOARCH`.

---

## 6) Payloads de teste (Console → SQS → _Send and receive messages_)

**Ordem por linguagem:** **WARMUP → SMALL → LARGE** (_aguarde esvaziar a fila antes do próximo_).

**Warmup (descartar)**

```json
{ "batch_id": "py-warm-001", "size_bytes": 1024, "count": 1, "label": "WARMUP" }
```

**Small (1 KB × 100)**

```json
{
	"batch_id": "py-small-1KBx100",
	"size_bytes": 1024,
	"count": 100,
	"label": "SMALL_1KBx100"
}
```

**Large (64 KB × 100)**

```json
{
	"batch_id": "py-large-64KBx100",
	"size_bytes": 65536,
	"count": 100,
	"label": "LARGE_64KBx100"
}
```

> Para Go, troque o prefixo do `batch_id` para `go-...`.  
> Se quiser uma fila única, use **Message attributes** com `lang="python"` ou `lang="go"` e configure o **filtro** no _trigger_.

---

## 7) Coleta das métricas (CloudWatch Logs → **Logs Insights**)

Selecione o **log group** da função (ex.: `/aws/lambda/tcc-python` ou `/aws/lambda/tcc-go`) e rode as consultas abaixo (_InsightsQL_).

### 7.1 Listar execuções com campos (REPORT)

```sql
fields @timestamp, @message
| filter @message like /REPORT/
| parse @message /REPORT RequestId: (?<req_id>[0-9a-f-]+)\s+Duration: (?<dur_ms>[\d.]+) ms\s+Billed Duration: (?<billed_ms>[\d.]+) ms(?:\s+Init Duration: (?<init_ms>[\d.]+) ms)?\s+Memory Size: (?<mem_mb>\d+) MB/
| sort @timestamp asc
| fields @timestamp, req_id, dur_ms, billed_ms, init_ms, mem_mb
```

> A primeira linha (com **Init Duration**) é o **WARMUP** → descarte. As duas seguintes são **SMALL** e **LARGE**, na ordem enviada.

### 7.2 Estatísticas do período

```sql
fields @message
| filter @message like /REPORT/
| parse @message /Duration: (?<dur_ms>[\d.]+) ms/
| parse @message /Billed Duration: (?<billed_ms>[\d.]+) ms/
| stats
    avg(dur_ms) as p50_ms,
    pct(dur_ms, 95) as p95_ms,
    avg(billed_ms) as billed_p50_ms,
    pct(billed_ms, 95) as billed_p95_ms,
    count(*) as invocations
```

> Use **Export results (CSV)** para salvar as leituras.

---

## 8) Estimativa de custo (fórmulas)

**Lambda (compute)**

```
GB-s = (Billed_ms / 1000) × (Memory_MB / 1024)
Custo_Lambda = GB-s × 0.0000166667 + (1 × 0.20 / 1_000_000)
```

**DynamoDB (on-demand, WRU)**

```
WRU por item ≈ ceil(size_bytes / 1024)
Custo_DDB = (WRU_por_item × count) × (0.625 / 1_000_000)
```

**Total por execução**

```
Total = Custo_Lambda + Custo_DDB
Custo por item = Total / count
```

> **Dica**: com **64 KB**, cada item consome ~**64 WRUs** → o custo de **DynamoDB** tende a **dominar**.  
> **Graviton/ARM**: multiplique a parte de **compute** do Lambda por ~**0,8** para estimar a economia típica.

---

## 9) Troubleshooting rápido

- **Runtime.HandlerNotFound (Python)**: verifique `Handler = lambda_function.handler` e arquivo `lambda_function.py` no ZIP.
- **Handler not found (Go)**: verifique `Runtime = provided.al2` e `Handler = bootstrap`; ZIP com `bootstrap` na **raiz**.
- **AccessDeniedException no PutItem**: confirme a **Role** com `dynamodb:PutItem` nas tabelas.
- **Erro ao criar trigger SQS**: adicione `sqs:ReceiveMessage / DeleteMessage / GetQueueAttributes` na Role.
- **Duplicatas no DynamoDB**: garanta `ConditionExpression` e `sk = "seq#<n>"` (**idempotência**).
- **Exec format error**: a **arquitetura** da Lambda (`arm64` vs `amd64`) não bate com o binário Go.
- **Boto3 ausente (Python 3.12)**: inclua o `boto3` no pacote ou use runtime **3.11**.

---

## 10) Pastas/Arquivos sugeridos no repositório

```
/src/python/lambda_function.py
/src/go/main.go
/docs/architecture_infrastructure.png
/results/metrics.csv           # exportado do CloudWatch
/README.md
```

---

## 11) Licença e citação

Código de exemplo distribuído no repositório do TCC.  
Ao citar este trabalho, referencie o repositório e a versão do documento do TCC:  
<https://github.com/MarceloSerra/tcc-usp-esalq>

---

## ✍️ Notas finais

- Mantenha **mesma memória** e **mesma arquitetura** nas duas funções.
- Execute **WARMUP → SMALL → LARGE** em **cada linguagem**, **esperando a fila esvaziar** entre cargas.
- Documente **região, memória, lote, chaves e idempotência** na sua metodologia para **reprodutibilidade**.
