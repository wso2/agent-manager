import json
import hashlib
from pathlib import Path

import logging
from langchain_community.document_loaders import PyPDFLoader
from langchain_text_splitters import RecursiveCharacterTextSplitter
from langchain_openai import OpenAIEmbeddings
from langchain_pinecone import PineconeVectorStore
from pinecone import Pinecone

from pydantic import ValidationError

from config import Settings, get_settings

logger = logging.getLogger(__name__)

DEFAULT_POLICIES_DIR = Path(__file__).resolve().parent / "resources" / "policy_pdfs"


class PolicyIngestion:
    def __init__(self, settings: Settings) -> None:
        self._settings = settings
        self._pdf_loader_cls = PyPDFLoader
        self._splitter = RecursiveCharacterTextSplitter(
            chunk_size=1000,
            chunk_overlap=200,
        )
        embeddings = OpenAIEmbeddings(
            model=self._settings.openai_embedding_model,
            api_key=self._settings.openai_api_key,
        )
        self._vectorstore = PineconeVectorStore(
            index_name=self._settings.pinecone_index_name,
            embedding=embeddings,
            pinecone_api_key=self._settings.pinecone_api_key,
            host=self._settings.pinecone_service_url,
        )

    def ingest_all_policies(self, policies_dir: Path) -> None:
        policies_root = policies_dir
        for hotel_dir in policies_root.iterdir():
            if hotel_dir.is_dir():
                self._ingest_policy_folder(hotel_dir)

    def _ingest_policy_folder(self, folder: Path) -> None:
        pdf_path = folder / "policies.pdf"
        metadata_path = folder / "metadata.json"

        if not pdf_path.exists() or not metadata_path.exists():
            logger.warning("Skipping %s: missing files", folder.name)
            return

        docs = self._pdf_loader_cls(str(pdf_path)).load()
        metadata = json.loads(metadata_path.read_text())
        if not metadata.get("hotel_id") or not metadata.get("hotel_name"):
            raise ValueError(
                f"Missing required hotel metadata in {metadata_path}. "
                "Expected hotel_id and hotel_name."
            )

        source_id = folder.name
        doc_type = metadata.get("doc_type", "policy")

        chunks = self._splitter.split_documents(docs)
        ids: list[str] = []
        for chunk in chunks:
            page = chunk.metadata.get("page")
            checksum = hashlib.sha256(chunk.page_content.encode("utf-8")).hexdigest()
            stable_id = f"{source_id}:{page}:{checksum}"
            chunk.metadata = {
                "source_id": source_id,
                "hotel_id": metadata.get("hotel_id"),
                "hotel_name": metadata.get("hotel_name"),
                "doc_type": doc_type,
                "page": page,
                "chunk_id": stable_id,
                "checksum": checksum,
            }
            ids.append(stable_id)
        self._vectorstore.add_documents(chunks, ids=ids)
        logger.info("Ingested %s", folder.name)


def ensure_policy_index() -> None:
    try:
        settings = get_settings()
    except ValidationError as exc:
        logger.info(
            "policy ingest skipped; invalid Pinecone settings: %s",
            exc,
        )
        return

    if not settings.pinecone_api_key or not settings.pinecone_service_url or not settings.pinecone_index_name:
        logger.info("policy ingest skipped; missing Pinecone settings.")
        return

    index_name = settings.pinecone_index_name
    try:
        pc = Pinecone(api_key=settings.pinecone_api_key)
        index_names = pc.list_indexes().names()
        if index_name not in index_names:
            logger.error(
                "policy ingest skipped; Pinecone index '%s' does not exist",
                index_name,
            )
            return
        stats = pc.Index(index_name).describe_index_stats()
        total_vectors = getattr(stats, "total_vector_count", 0)
        if total_vectors > 0:
            logger.info(
                "policy index '%s' already has %s vectors; skipping ingest",
                index_name,
                total_vectors,
            )
            return
        logger.info(
            "policy index '%s' exists but is empty; proceeding with ingest",
            index_name,
        )
    except Exception:
        logger.exception("failed to check Pinecone index; skipping policy ingest")
        return

    policies_dir = Path(settings.policies_dirs) if settings.policies_dirs else DEFAULT_POLICIES_DIR
    try:
        ingestion = PolicyIngestion(settings)
        ingestion.ingest_all_policies(policies_dir=policies_dir)
        logger.info("policy ingest completed")
    except Exception:
        logger.exception("policy ingest failed")


if __name__ == "__main__":
    ensure_policy_index()
