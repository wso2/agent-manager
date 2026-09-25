from __future__ import annotations

import logging

from fastapi import FastAPI

from booking import router as booking_router
from policies import init_policy_store, router as policies_router
from search import router as search_router

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)

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
