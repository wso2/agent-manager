import json
import os
from pathlib import Path

import logging
from typing import Any

from dotenv import load_dotenv
from langchain_community.document_loaders import PyPDFLoader
from langchain_text_splitters import RecursiveCharacterTextSplitter
from langchain_openai import OpenAIEmbeddings
from langchain_pinecone import PineconeVectorStore
from pinecone import Pinecone

load_dotenv()

logger = logging.getLogger(__name__)

DEFAULT_POLICIES_DIR = Path(__file__).resolve().parent / "resources" / "policy_pdfs"


class PolicyIngestion:
    def __init__(self) -> None:
        self._pdf_loader_cls = PyPDFLoader
        self._splitter = RecursiveCharacterTextSplitter(
            chunk_size=1000,
            chunk_overlap=200,
        )
        embeddings = OpenAIEmbeddings(
            model=os.getenv("OPENAI_EMBEDDING_MODEL")
        )
        self._vectorstore = PineconeVectorStore(
            index_name=os.getenv("PINECONE_INDEX_NAME"),
            embedding=embeddings,
            pinecone_api_key=os.getenv("PINECONE_API_KEY"),
            pinecone_host=os.getenv("PINECONE_SERVICE_URL"),
        )


    def ingest_all_policies(self, policies_dir: str | Path) -> None:
        policies_root = Path(policies_dir)
        for hotel_dir in policies_root.iterdir():
            if hotel_dir.is_dir():
                self._ingest_policy_folder(hotel_dir)

    def _ingest_policy_folder(self, folder: Path) -> None:
        pdf_path = folder / "policies.pdf"
        metadata_path = folder / "metadata.json"

        if not pdf_path.exists() or not metadata_path.exists():
            print(f"Skipping {folder.name}: missing files")
            return

        docs = self._pdf_loader_cls(str(pdf_path)).load()

        metadata = json.loads(metadata_path.read_text())

        for d in docs:
            d.metadata.update(metadata)
            d.metadata["source"] = folder.name

        chunks = self._splitter.split_documents(docs)
        self._vectorstore.add_documents(chunks)
        print(f"✓ Ingested {folder.name}")

def ensure_policy_index() -> None:
    pinecone_api_key = os.getenv("PINECONE_API_KEY")
    if not pinecone_api_key:
        logger.info("PINECONE_API_KEY not set; skipping policy ingest")
        return

    index_name = os.getenv("PINECONE_INDEX_NAME", "hotelbookingdb")
    try:
        pc = Pinecone(api_key=pinecone_api_key)
        index_names = pc.list_indexes().names()
        if index_name in index_names:
            logger.info("policy index '%s' already exists; skipping ingest", index_name)
            return
    except Exception:
        logger.exception("failed to check Pinecone index; skipping policy ingest")
        return

    policies_dir = os.getenv("POLICIES_DIRS") or str(DEFAULT_POLICIES_DIR)
    try:
        ingestion = PolicyIngestion()
        ingestion.ingest_all_policies(policies_dir=policies_dir)
        logger.info("policy ingest completed")
    except Exception:
        logger.exception("policy ingest failed")


if __name__ == "__main__":
    ensure_policy_index()
