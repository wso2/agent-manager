import json
from pathlib import Path

import logging
from langchain_community.document_loaders import PyPDFLoader
from langchain_text_splitters import RecursiveCharacterTextSplitter
from langchain_openai import OpenAIEmbeddings
from langchain_pinecone import PineconeVectorStore
from pinecone import Pinecone

from pydantic import ValidationError

from config import Settings

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
        embeddings = OpenAIEmbeddings(model=self._settings.openai_embedding_model)
        self._vectorstore = PineconeVectorStore(
            index_name=self._settings.pinecone_index_name,
            embedding=embeddings,
            pinecone_api_key=self._settings.pinecone_api_key,
            pinecone_host=self._settings.pinecone_service_url,
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

        for d in docs:
            d.metadata.update(metadata)
            d.metadata["source"] = folder.name

        chunks = self._splitter.split_documents(docs)
        self._vectorstore.add_documents(chunks)
        logger.info("Ingested %s", folder.name)


def ensure_policy_index() -> None:
    try:
        settings = Settings()
    except ValidationError as exc:
        logger.info(
            "policy ingest skipped; missing Pinecone settings: %s",
            exc,
        )
        return

    index_name = settings.pinecone_index_name
    try:
        pc = Pinecone(api_key=settings.pinecone_api_key)
        index_names = pc.list_indexes().names()
        if index_name in index_names:
            stats = pc.Index(index_name).describe_index_stats()
            total_vectors = stats.get("total_vector_count", 0)
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
