import os
import json
from pathlib import Path
from typing import List, Dict, Any
from dotenv import load_dotenv
from pypdf import PdfReader
from pinecone import Pinecone
from openai import OpenAI

# Load environment variables
load_dotenv()

class DocumentChunker:
    """Handles recursive document chunking with overlap"""
    
    def __init__(self, chunk_size: int = 1000, chunk_overlap: int = 200):
        self.chunk_size = chunk_size
        self.chunk_overlap = chunk_overlap
    
    def chunk_text(self, text: str, metadata: Dict[str, Any]) -> List[Dict[str, Any]]:
        """
        Recursively chunk text into smaller segments with overlap
        """
        chunks = []
        text_length = len(text)
        
        if text_length == 0:
            return chunks
        
        start = 0
        chunk_id = 0
        
        while start < text_length:
            end = start + self.chunk_size
            chunk_text = text[start:end]
            
            chunks.append({
                "content": chunk_text.strip(),
                "metadata": {
                    **metadata,
                    "chunk_id": chunk_id,
                    "start_index": start,
                    "end_index": min(end, text_length)
                }
            })
            
            chunk_id += 1
            start = end - self.chunk_overlap
            
            # Prevent infinite loop
            if start >= text_length:
                break
        
        return chunks


class PolicyIngestion:
    """Main class for ingesting policy documents into Pinecone"""
    
    def __init__(self):
        # Initialize OpenAI client for embeddings
        self.openai_client = OpenAI(api_key=os.getenv("OPENAI_API_KEY"))
        
        # Initialize Pinecone
        pc = Pinecone(api_key=os.getenv("PINECONE_API_KEY"))
        self.index = pc.Index(
            os.getenv("PINECONE_INDEX_NAME", "hotel-policies"),
            host=os.getenv("PINECONE_SERVICE_URL")
        )
        
        # Initialize chunker
        self.chunker = DocumentChunker(
            chunk_size=int(os.getenv("CHUNK_SIZE", "1000")),
            chunk_overlap=int(os.getenv("CHUNK_OVERLAP", "200"))
        )
    
    def extract_text_from_pdf(self, pdf_path: str) -> str:
        """Extract text from PDF file"""
        try:
            reader = PdfReader(pdf_path)
            text = ""
            for page in reader.pages:
                text += page.extract_text() + "\n"
            return text
        except Exception as e:
            raise Exception(f"Error reading PDF {pdf_path}: {str(e)}")
    
    def load_metadata(self, metadata_path: str) -> Dict[str, Any]:
        """Load metadata from JSON file"""
        try:
            with open(metadata_path, 'r', encoding='utf-8') as f:
                return json.load(f)
        except Exception as e:
            raise Exception(f"Error reading metadata {metadata_path}: {str(e)}")
    
    def generate_embedding(self, text: str) -> List[float]:
        """Generate embeddings using OpenAI"""
        try:
            model_name = os.getenv("OPENAI_EMBEDDING_MODEL", "text-embedding-3-small")
            response = self.openai_client.embeddings.create(
                model=model_name,
                input=text
            )
            return response.data[0].embedding
        except Exception as e:
            raise Exception(f"Error generating embedding: {str(e)}")
    
    def ingest_chunks(self, chunks: List[Dict[str, Any]], source_folder: str):
        """Ingest document chunks into Pinecone"""
        vectors = []
        
        for i, chunk in enumerate(chunks):
            # Generate embedding for chunk content
            embedding = self.generate_embedding(chunk["content"])
            
            # Create unique ID for the chunk
            chunk_id = f"{source_folder.replace('/', '_')}_{i}"
            
            # Prepare vector for Pinecone
            vectors.append({
                "id": chunk_id,
                "values": embedding,
                "metadata": {
                    "content": chunk["content"],
                    **chunk["metadata"]
                }
            })
        
        # Batch upsert to Pinecone
        if vectors:
            self.index.upsert(vectors=vectors)
    
    def process_policy_folder(self, folder_path: Path):
        """Process a single policy folder"""
        policy_pdf_path = folder_path / "policies.pdf"
        metadata_json_path = folder_path / "metadata.json"
        
        # Check if required files exist
        if not policy_pdf_path.exists():
            print(f"Warning: policies.pdf not found in {folder_path}")
            return
        
        if not metadata_json_path.exists():
            print(f"Warning: metadata.json not found in {folder_path}")
            return
        
        try:
            # Extract text from PDF
            print(f"Processing: {folder_path}")
            hotel_policy = self.extract_text_from_pdf(str(policy_pdf_path))
            
            # Load metadata
            metadata = self.load_metadata(str(metadata_json_path))
            
            # Chunk the document
            chunks = self.chunker.chunk_text(hotel_policy, metadata)
            
            # Ingest chunks into Pinecone
            self.ingest_chunks(chunks, folder_path.name)
            
            print(f"Successfully ingested policy from: {folder_path}")
            
        except Exception as e:
            print(f"Error processing {folder_path}: {str(e)}")
            raise
    
    def _default_policy_dirs(self) -> List[Path]:
        repo_root = Path(__file__).resolve().parents[2]
        return [repo_root / "resources/policy_pdfs"]

    def ingest_all_policies(self, policies_dir: str | None = None):
        """Main method to ingest all policy documents"""
        raw_dirs = policies_dir or os.getenv("POLICIES_DIRS", "")
        if raw_dirs:
            policies_paths = [Path(p.strip()) for p in raw_dirs.split(",") if p.strip()]
        else:
            policies_paths = self._default_policy_dirs()

        for policies_path in policies_paths:
            if not policies_path.exists():
                print(f"Warning: policies directory not found: {policies_path}")
                continue
            for entry in policies_path.iterdir():
                if entry.is_dir():
                    self.process_policy_folder(entry)


def main():
    try:
        ingestion = PolicyIngestion()
        ingestion.ingest_all_policies()
        print("\n✓ All policies successfully ingested!")
        
    except Exception as e:
        print(f"\n✗ Error occurred: {str(e)}")
        raise


if __name__ == "__main__":
    main()