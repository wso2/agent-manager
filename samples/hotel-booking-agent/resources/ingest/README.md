# Python Ingest

Ingest flow for hotel policy PDFs. It reads `resources/policy_pdfs/**/policies.pdf`
with `resources/policy_pdfs/**/metadata.json`, chunks the text, generates embeddings, and upserts to Pinecone.

## Setup

```bash
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

## Configuration

Set the following environment variables in `resources/ingest/.env` (separate from the agent `.env`). You can copy `resources/ingest/.env.example` as a starting point.

```bash
PINECONE_SERVICE_URL="https://your-index-xxxxxx.svc.your-region.pinecone.io"
PINECONE_API_KEY="your-pinecone-api-key"
PINECONE_NAMESPACE=""
OPENAI_API_KEY="your-openai-api-key"
OPENAI_EMBEDDING_MODEL="text-embedding-3-small"
PINECONE_INDEX_NAME="hotel-policies"
POLICIES_DIRS="/absolute/path/to/resources/policy_pdfs"
SINGLE_PDF_PATH="/absolute/path/to/Hotel_Policies_Document.pdf"
SINGLE_PDF_DEFAULT_HOTEL="Unknown Hotel"
CHUNK_SIZE=1000
CHUNK_OVERLAP=200
```

## Run

```bash
python ingest.py
```

Notes:
- By default the script ingests from `resources/policy_pdfs`.
- Use `POLICIES_DIRS` (comma-separated) to point at other policy folders.
- Use `SINGLE_PDF_PATH` to ingest one PDF that contains multiple hotels. The script will split on headings like `Hotel Name - Hotel Policies` or lines like `Hotel: Name`.
- If no hotel name is found on a page, it will use `SINGLE_PDF_DEFAULT_HOTEL`.
- Chunk size and overlap can be overridden with `CHUNK_SIZE` and `CHUNK_OVERLAP`.
