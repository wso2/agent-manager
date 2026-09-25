import json
import hashlib
from pathlib import Path

import logging
from langchain_community.document_loaders import PyPDFLoader
from langchain_core.vectorstores import VectorStore
from langchain_text_splitters import RecursiveCharacterTextSplitter
from langchain_openai import OpenAIEmbeddings
from langchain_pinecone import PineconeVectorStore
from pinecone import Pinecone, ServerlessSpec

from pydantic import ValidationError

from config import Settings, get_settings

logger = logging.getLogger(__name__)

DEFAULT_POLICIES_DIR = Path(__file__).resolve().parent / "resources" / "policy_pdfs"


def build_embeddings(settings: Settings) -> OpenAIEmbeddings:
    return OpenAIEmbeddings(
        model=settings.openai_embedding_model,
        api_key=settings.openai_api_key,
    )


class PolicyIngestion:
    def __init__(self, vectorstore: VectorStore) -> None:
        self._vectorstore = vectorstore
        self._pdf_loader_cls = PyPDFLoader
        self._splitter = RecursiveCharacterTextSplitter(
            chunk_size=1000,
            chunk_overlap=200,
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
        logger.info("ingesting %s: %s pages -> %s chunks", folder.name, len(docs), len(chunks))
        self._vectorstore.add_documents(chunks, ids=ids)
        logger.info("ingested %s", folder.name)


def ensure_policy_index() -> PineconeVectorStore | None:
    try:
        settings = get_settings()
    except ValidationError as exc:
        logger.info(
            "policy ingest skipped; invalid Pinecone settings: %s",
            exc,
        )
        return None

    if not settings.pinecone_api_key:
        logger.info("policy ingest skipped; PINECONE_API_KEY not set.")
        return None

    index_name = settings.pinecone_index_name
    logger.info(
        "Pinecone config: index=%s host=%s",
        index_name,
        settings.pinecone_service_url or "(looked up by index name)",
    )
    try:
        pc = Pinecone(api_key=settings.pinecone_api_key)
        if settings.pinecone_service_url:
            index = pc.Index(host=settings.pinecone_service_url)
        else:
            index_names = pc.list_indexes().names()
            logger.info("Pinecone indexes in this project: %s", list(index_names) or "none")
            if index_name not in index_names:
                dimension = len(build_embeddings(settings).embed_query("dimension probe"))
                logger.info("creating Pinecone index '%s' (dimension %s)", index_name, dimension)
                pc.create_index(
                    name=index_name,
                    dimension=dimension,
                    metric="cosine",
                    spec=ServerlessSpec(cloud="aws", region="us-east-1"),
                )
                logger.info("created Pinecone index '%s'", index_name)
            index = pc.Index(index_name)
        vectorstore = PineconeVectorStore(index=index, embedding=build_embeddings(settings))
        total_vectors = getattr(index.describe_index_stats(), "total_vector_count", 0)
    except Exception:
        logger.exception("failed to prepare Pinecone index; policy search disabled")
        return None

    policies_dir = Path(settings.policies_dirs) if settings.policies_dirs else DEFAULT_POLICIES_DIR
    # Chunk IDs are stable, so re-ingesting on every start upserts without duplicates.
    logger.info(
        "policy index '%s' has %s vectors; upserting policies from %s",
        index_name,
        total_vectors,
        policies_dir,
    )
    try:
        PolicyIngestion(vectorstore).ingest_all_policies(policies_dir=policies_dir)
        logger.info("policy ingest completed")
    except Exception:
        logger.exception("policy ingest failed")
    return vectorstore


if __name__ == "__main__":
    ensure_policy_index()
