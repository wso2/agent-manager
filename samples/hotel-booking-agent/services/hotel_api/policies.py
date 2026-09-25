from __future__ import annotations

import logging
from pathlib import Path

from fastapi import APIRouter, Query, status
from fastapi.responses import JSONResponse
from langchain_core.vectorstores import InMemoryVectorStore, VectorStore

from config import get_settings
from ingest import DEFAULT_POLICIES_DIR, PolicyIngestion, build_embeddings, ensure_policy_index

logger = logging.getLogger(__name__)

router = APIRouter()

_store: VectorStore | None = None
_backend: str | None = None


def init_policy_store() -> None:
    global _store, _backend
    settings = get_settings()
    if not settings.openai_api_key:
        logger.warning("policy search disabled: OPENAI_API_KEY not set")
        return

    if settings.pinecone_api_key:
        _store = ensure_policy_index()
        _backend = "pinecone" if _store else None
        return

    policies_dir = Path(settings.policies_dirs) if settings.policies_dirs else DEFAULT_POLICIES_DIR
    logger.info("PINECONE_API_KEY not set; loading policies into an in-memory store from %s", policies_dir)
    try:
        store = InMemoryVectorStore(build_embeddings(settings))
        PolicyIngestion(store).ingest_all_policies(policies_dir=policies_dir)
    except Exception:
        logger.exception("in-memory policy ingest failed; policy search disabled")
        return
    _store, _backend = store, "in-memory"
    logger.info("policy search ready (in-memory)")


@router.get("/hotels/{hotel_id}/policies/search")
def search_policies_route(hotel_id: str, q: str, k: int = Query(5, ge=1, le=20)):
    if _store is None:
        return JSONResponse(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            content={"message": "Policy search is not configured", "error_code": "POLICY_SEARCH_UNAVAILABLE"},
        )
    if _backend == "pinecone":
        search_filter = {"hotel_id": {"$eq": hotel_id}}
    else:
        search_filter = lambda doc: doc.metadata.get("hotel_id") == hotel_id
    try:
        docs = _store.similarity_search(q, k=k, filter=search_filter)
    except Exception:
        logger.exception("policy search failed: hotel_id=%s", hotel_id)
        return JSONResponse(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            content={"message": "Policy search failed", "error_code": "POLICY_SEARCH_FAILED"},
        )
    logger.info("policy search (%s) returned %s results for hotel_id=%s", _backend, len(docs), hotel_id)
    return {
        "hotel_id": hotel_id,
        "source": _backend,
        "results": [{"text": d.page_content, "page": d.metadata.get("page")} for d in docs],
    }
