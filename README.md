# Planet-Caravan

## Gin project

```bash
cd /home/runner/work/Planet-Caravan/Planet-Caravan/gin
go mod tidy
go run .
```

Health check: `GET http://localhost:8080/health`

## FastAPI project

```bash
cd /home/runner/work/Planet-Caravan/Planet-Caravan/fastapi
python -m pip install -r requirements.txt
uvicorn main:app --reload
```

Health check: `GET http://127.0.0.1:8000/health`
