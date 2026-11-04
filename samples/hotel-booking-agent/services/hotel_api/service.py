from __future__ import annotations

from fastapi import FastAPI

from booking import router as booking_router
from logging_config import configure_logging
from policies import init_policy_store, router as policies_router
from search import router as search_router

configure_logging()

app = FastAPI(title="Hotel Booking API")


@app.get("/health")
def health():
    return {"status": "ok"}


app.include_router(booking_router)
app.include_router(search_router)
app.include_router(policies_router)


@app.on_event("startup")
def bootstrap_policy_store() -> None:
    init_policy_store()
