from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict

def _split_csv(value: str | None, default: list[str]) -> list[str]:
    if value is None:
        return default
    stripped = [item.strip() for item in value.split(",")]
    return [item for item in stripped if item]


class Settings(BaseSettings):
    openai_api_key: str
    openai_model: str = "gpt-4o-mini"
    openai_embedding_model: str = "text-embedding-3-small"
    pinecone_api_key: str
    pinecone_service_url: str
    pinecone_index_name: str = "hotel-policies"
    weather_api_key: str | None = None
    weather_api_base_url: str = "http://api.weatherapi.com/v1"
    booking_api_base_url: str = "http://localhost:9091"
    cors_allow_origins: list[str] | str = Field(default_factory=lambda: ["http://localhost:3001"])
    cors_allow_credentials: bool = True

    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    @property
    def cors_allow_origins_list(self) -> list[str]:
        if isinstance(self.cors_allow_origins, list):
            return self.cors_allow_origins
        return _split_csv(self.cors_allow_origins, ["http://localhost:3001"])
