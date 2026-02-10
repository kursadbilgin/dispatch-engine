# Postman Test Flow

## Files
- `dispatch-engine.postman_collection.json`: API istekleri + pre-request/test scriptleri.
- `dispatch-engine.local.postman_environment.json`: Lokal environment (`baseUrl=http://localhost:8080`).

## Run in Postman
1. Collection ve environment dosyalarını Postman'a import et.
2. Environment olarak `dispatch-engine-local` seç.
3. Collection içindeki `E2E Flow` klasörünü Runner ile çalıştır.
4. Polling adımı (`GET /v1/notifications/{id} (poll)`) otomatik döner, terminal state'e gelince sonraki adıma geçer.

## What is validated
- `livez` / `readyz` sağlık kontrolleri
- Tekil notification enqueue + durum polling
- Batch enqueue + batch durum sorgulama
- Notification listeleme (normal + filtered)
- Prometheus `/metrics` metrik varlığı
- Opsiyonel cancel endpointi (`200` veya `409`)
