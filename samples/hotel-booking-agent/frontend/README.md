# Travel Planner Frontend

React UI for the Lab 02 travel planner demo. It talks to the agent `/travelPlanner/chat` endpoint and the hotel APIs.

## Prerequisites

- Node.js 22+
- npm

## Configuration

Update `frontend/.env` as needed:

```bash
PORT=3001
REACT_APP_CHAT_API_URL=http://localhost:9090/chat
REACT_APP_API_BASE_URL=http://localhost:9090
REACT_APP_ASGARDEO_CLIENT_ID=...
REACT_APP_ASGARDEO_BASE_URL=https://api.asgardeo.io/t/<org>
REACT_APP_ASGARDEO_SCOPES=openid profile email
```

`REACT_APP_CHAT_API_URL` defaults to `http://localhost:9090/chat`.
`REACT_APP_API_BASE_URL` is the FastAPI agent base URL for booking/profile endpoints.
`REACT_APP_ASGARDEO_CLIENT_ID` and `REACT_APP_ASGARDEO_BASE_URL` come from your Asgardeo app settings.

## Run

```bash
cd frontend
npm install
npm start
```

The app will be available at `http://localhost:3001`.
