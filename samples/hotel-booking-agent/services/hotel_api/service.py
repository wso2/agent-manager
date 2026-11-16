from __future__ import annotations

from secrets import compare_digest
from typing import Annotated

from fastapi import Depends, FastAPI, HTTPException, Security
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer

from booking import router as booking_router
from config import get_settings
from logging_config import configure_logging
from policies import init_policy_store, router as policies_router
from search import router as search_router

configure_logging()

app = FastAPI(title="Hotel Booking API")
bearer = HTTPBearer(scheme_name="hotelApiBearer", auto_error=False)


def require_hotel_api_token(
    credentials: Annotated[HTTPAuthorizationCredentials | None, Security(bearer)],
) -> None:
    token = get_settings().hotel_api_token.get_secret_value()
    if credentials is None or not compare_digest(credentials.credentials.encode(), token.encode()):
        raise HTTPException(
            status_code=401,
            detail="Invalid or missing Hotel API token",
            headers={"WWW-Authenticate": "Bearer"},
        )


@app.get("/health")
def health():
    return {"status": "ok"}


app.include_router(booking_router, dependencies=[Depends(require_hotel_api_token)])
app.include_router(search_router, dependencies=[Depends(require_hotel_api_token)])
app.include_router(policies_router, dependencies=[Depends(require_hotel_api_token)])


@app.on_event("startup")
def bootstrap_policy_store() -> None:
    get_settings()
    init_policy_store()
