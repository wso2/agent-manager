from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    openai_api_key: str
    openai_model: str = "gpt-4o-mini"
    openai_embedding_model: str = "text-embedding-3-small"
    openai_timeout: float = Field(
        default=30.0,
        description="Timeout in seconds for OpenAI API calls.",
    )
    openai_max_retries: int = Field(
        default=3,
        description="Maximum retry attempts for OpenAI API calls.",
    )
    pinecone_api_key: str
    pinecone_service_url: str
    pinecone_index_name: str = "hotel-policies"
    weather_api_key: str | None = None
    weather_api_base_url: str = "http://api.weatherapi.com/v1"
    hotel_api_base_url: str = Field(
        default="http://localhost:9091",
        description="Base URL for the hotel booking API.",
        validation_alias="HOTEL_API_BASE_URL",
    )
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

settings = Settings()
