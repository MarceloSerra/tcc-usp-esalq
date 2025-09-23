## Arquitetura infraestrutura
![Arquitetura infraestrutura](./diagram/architecture_infrastructure.png)


## Garantindo “fairness” entre Python e Go

Mesma arquitetura (ambas arm64 ou ambas x86_64).

Mesma memória e mesmo timeout.

Sem VPC (a menos que precise; VPC adiciona latência de ENI).

Provisioned Concurrency desligado (a menos que esteja comparando aquecido).

Rode primeiro 1 chamada “quente” descartável para aquecer; depois colete séries.

Execute os mesmos lotes (mesmos size_bytes, count, reps) nas duas linguagens.

Faça várias repetições (≥5) e reporte média + p95/p99.


## Checklist rápido

Tabelas: tcc_lambda_python, tcc_lambda_go (On-Demand, PK/SK).

Role IAM: tcc-lambda-dynamo-role com CloudWatch Logs + DynamoDB.

Lambda tcc-python (env TABLE_NAME), memória/timeout iguais aos do Go.

Lambda tcc-go (zip do binário), env TABLE_NAME, mesma memória/timeout.

Function URL em ambas e script driver.py para disparar lotes.

Exportar tabelas ao S3 + baixar.

CloudWatch Logs Insights: exportar CSV com Billed/Duration.

Rodar analisar.py (ou sua planilha) e montar tabelas do TCC (média, p95, custos).
