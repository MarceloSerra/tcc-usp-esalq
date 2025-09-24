import os, time, uuid, json
import boto3

dynamodb = boto3.resource('dynamodb')
TABLE = dynamodb.Table(os.environ['TABLE_NAME'])
COLD = True

def _now_ms(): return int(time.time()*1000)

def process_one(e, context):
    global COLD
    batch_id = e.get("batch_id") or str(uuid.uuid4())
    size_bytes = int(e.get("size_bytes", 1024))
    count = int(e.get("count", 100))
    label = e.get("label", f"{size_bytes}Bx{count}")
    payload_str = "x" * size_bytes

    t_batch0 = _now_ms()
    for seq in range(count):
        t0 = _now_ms()
        TABLE.put_item(Item={
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
        })
    COLD = False
    return {"ok": True, "batch_elapsed_ms": _now_ms() - t_batch0}

def handler(event, context):
    # Se veio de SQS (mesmo com batch size=1 vem envelopado)
    if isinstance(event, dict) and "Records" in event and event["Records"]:
        failures = []
        for r in event["Records"]:
            try:
                body = json.loads(r["body"])
                process_one(body, context)
            except Exception:
                failures.append({"itemIdentifier": r["messageId"]})
        return {"batchItemFailures": failures}
    # Chamada direta (Function URL/Test do console)
    return process_one(event, context)

# 👇 Alias esperado pelo runtime padrão do console
def lambda_handler(event, context):
    return handler(event, context)
