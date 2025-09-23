import os, time, uuid
import boto3
from datetime import datetime
from aws_lambda_powertools.utilities.typing import LambdaContext  # opcional se tiver lib; se não, ignore

dynamodb = boto3.resource('dynamodb')
TABLE = dynamodb.Table(os.environ['TABLE_NAME'])

COLD = True  # flag simples pra marcar cold start por instância

def _now_ms():
    return int(time.time() * 1000)

def handler(event, context):  # type: ignore[override]
    global COLD
    batch_id = event.get("batch_id") or str(uuid.uuid4())
    size_bytes = int(event.get("size_bytes", 1024))
    count = int(event.get("count", 100))
    label = event.get("label", f"{size_bytes}Bx{count}")

    payload_str = "x" * size_bytes
    results = []
    t_batch_start = _now_ms()

    for seq in range(count):
        t0 = _now_ms()
        item = {
            "pk": f"batch_id#{batch_id}",
            "sk": f"ts#{t0}#{seq}",
            "lang": "python",
            "label": label,
            "size_bytes": size_bytes,
            "seq": seq,
            "t_start_ms": t0,
            "payload": payload_str,
            "cold_start": COLD,
            "mem_mb": context.memory_limit_in_mb,
            "request_id": context.aws_request_id,
        }
        # Escreve
        TABLE.put_item(Item=item)
        t1 = _now_ms()
        item["t_end_ms"] = t1
        item["elapsed_ms"] = t1 - t0
        results.append(item)

    t_batch_end = _now_ms()
    COLD = False

    # Resumo do lote (útil para ver no retorno/Logs)
    summary = {
        "batch_id": batch_id,
        "lang": "python",
        "label": label,
        "size_bytes": size_bytes,
        "count": count,
        "batch_elapsed_ms": t_batch_end - t_batch_start,
        "per_item_ms_avg": sum(r["elapsed_ms"] for r in results) / len(results),
    }
    return summary
