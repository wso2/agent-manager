import os
from dataclasses import dataclass
from dotenv import load_dotenv

load_dotenv()


@dataclass
class Settings:
    openai_api_key: str
    openai_model: str
    openai_embedding_model: str
    pinecone_api_key: str
    pinecone_service_url: str
    pinecone_index_name: str
    weather_api_key: str | None
    weather_api_base_url: str
    booking_api_base_url: str

    @classmethod
    def from_env(cls) -> "Settings":
        def required(name: str) -> str:
            value = os.getenv(name)
            if not value:
                raise ValueError(f"Missing required env var: {name}")
            return value

        return cls(
            openai_api_key=required("OPENAI_API_KEY"),
            openai_model=os.getenv("OPENAI_MODEL", "gpt-4o-mini"),
            openai_embedding_model=os.getenv("OPENAI_EMBEDDING_MODEL", "text-embedding-3-small"),
            pinecone_api_key=required("PINECONE_API_KEY"),
            pinecone_service_url=required("PINECONE_SERVICE_URL"),
            pinecone_index_name=os.getenv("PINECONE_INDEX_NAME", "hotel-policies"),
            weather_api_key=os.getenv("WEATHER_API_KEY"),
            weather_api_base_url=os.getenv("WEATHER_API_BASE_URL", "http://api.weatherapi.com/v1"),
            booking_api_base_url=os.getenv("BOOKING_API_BASE_URL", "http://localhost:9091"),
        )
