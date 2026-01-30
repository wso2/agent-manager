import os
from dataclasses import dataclass
from dotenv import load_dotenv

load_dotenv()

def _split_csv(value: str | None, default: list[str]) -> list[str]:
    if value is None:
        return default
    stripped = [item.strip() for item in value.split(",")]
    return [item for item in stripped if item]


@dataclass
class Settings:
    openai_api_key: str
    openai_model: str
    openai_embedding_model: str
    asgardeo_base_url: str
    asgardeo_client_id: str
    pinecone_api_key: str
    pinecone_service_url: str
    pinecone_index_name: str
    weather_api_key: str | None
    weather_api_base_url: str
    booking_api_base_url: str
    cors_allow_origins: list[str]
    cors_allow_credentials: bool

    @classmethod
    def from_env(cls) -> "Settings":
        def required(name: str) -> str:
            value = os.getenv(name)
            if not value:
                raise ValueError(f"Missing required env var: {name}")
            return value
        asgardeo_base_url = required("ASGARDEO_BASE_URL")
        asgardeo_client_id = required("ASGARDEO_CLIENT_ID")
        return cls(
            openai_api_key=required("OPENAI_API_KEY"),
            openai_model=os.getenv("OPENAI_MODEL", "gpt-4o-mini"),
            openai_embedding_model=os.getenv("OPENAI_EMBEDDING_MODEL", "text-embedding-3-small"),
            asgardeo_base_url=asgardeo_base_url,
            asgardeo_client_id=asgardeo_client_id,
            pinecone_api_key=required("PINECONE_API_KEY"),
            pinecone_service_url=required("PINECONE_SERVICE_URL"),
            pinecone_index_name=os.getenv("PINECONE_INDEX_NAME", "hotel-policies"),
            weather_api_key=os.getenv("WEATHER_API_KEY"),
            weather_api_base_url=os.getenv("WEATHER_API_BASE_URL", "http://api.weatherapi.com/v1"),
            booking_api_base_url=os.getenv("BOOKING_API_BASE_URL", "http://localhost:9091"),
            cors_allow_origins=_split_csv(
                os.getenv("CORS_ALLOW_ORIGINS"),
                ["http://localhost:3001"],
            ),
            cors_allow_credentials=os.getenv("CORS_ALLOW_CREDENTIALS", "true").lower() == "true",
        )
