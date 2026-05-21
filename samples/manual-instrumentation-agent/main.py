"""Local runner: ``python main.py`` serves the agent on port 8000."""

import uvicorn

from app import app

if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8000)
