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

Set the following environment variables (you can put them in a local `.env` file):

```bash
PINECONE_SERVICE_URL="https://your-index-xxxxxx.svc.your-region.pinecone.io"
PINECONE_API_KEY="your-pinecone-api-key"
OPENAI_API_KEY="your-openai-api-key"
PINECONE_INDEX_NAME="hotel-policies"
POLICIES_DIRS="/absolute/path/to/resources/policy_pdfs"
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
- Chunk size and overlap can be overridden with `CHUNK_SIZE` and `CHUNK_OVERLAP`.
