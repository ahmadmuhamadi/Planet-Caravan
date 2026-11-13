from fastapi import FastAPI

app = FastAPI(title="Planet Caravan compute")


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok"}
