"""Local runner: ``python main.py`` serves the A2A agent on port 9099."""

import uvicorn

from app import PORT, app

if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=PORT)
