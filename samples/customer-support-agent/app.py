import dotenv
from fastapi import FastAPI
from fastapi.responses import JSONResponse

from agent.agent import create_agent


app = FastAPI()
# Load environment variables from a .env file (if present)
dotenv.load_dotenv()
agent_graph = create_agent()


def run_agent(thread_id: int, question: str, passenger_id: str = "3442 587242"):
    config = {
        "configurable": {
            # The passenger_id is used in our flight tools to
            # fetch the user's flight information
            "passenger_id": passenger_id,
            # Checkpoints are accessed by thread_id
            "thread_id": thread_id,
        }
    }

    events = agent_graph.stream(
        {"messages": ("user", question)}, config, stream_mode="values"
    )

    final_answer = None
    for event in events:
        # Each event is a dict representing a streamed update
        if "messages" in event:
            print(f"Received answer: {event}")
            # Keep updating until we get the latest assistant message
            final_answer = event["messages"][-1].content

    return final_answer


@app.post("/chat")
async def chat(payload: dict):
    # Process the payload as needed
    result = {
        "response": run_agent(
            payload["session_id"],
            payload["message"],
        )
    }
    return JSONResponse(content=result)
