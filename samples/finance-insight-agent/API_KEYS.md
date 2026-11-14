# API Key Management

## Quick Links

| Service | Purpose | Get API Key | Free Tier |
|---------|---------|-------------|-----------|
| **OpenAI** | LLM for agents (Required) | [platform.openai.com/api-keys](https://platform.openai.com/api-keys) | $5 free credit |
| **SerpAPI** | Google search for news (Required) | [serpapi.com](https://serpapi.com) | 250 searches/month |
| **Alpha Vantage** | Company fundamentals (Required for ratios) | [alphavantage.co/support/#api-key](https://www.alphavantage.co/support/#api-key) | 25 requests/day |
| **Twelve Data** | Market data (Required) | [twelvedata.com/apikey](https://twelvedata.com/apikey) | Credits/day (varies by endpoint) |

---

## How to Get API Keys

### 1. OpenAI API Key (Required)

**Step-by-step:**
1. Go to [platform.openai.com](https://platform.openai.com)
2. Sign up or log in to your account
3. Click your profile icon → **View API Keys**
4. Click **Create new secret key**
5. Name it "Finance Insight Service"
6. Copy the key (starts with `sk-proj-...`)
7. **Save it immediately** - you won't see it again!

**Pricing:**
- GPT-4o: $2.50 / 1M input tokens, $10 / 1M output tokens
- GPT-4o-mini: $0.15 / 1M input tokens, $0.60 / 1M output tokens
- Average query cost: ~$0.10-0.30 with GPT-4o

**Important:** Add billing information at [platform.openai.com/settings/billing](https://platform.openai.com/settings/billing) to increase rate limits.

### 2. SerpAPI Key (Required)

**Step-by-step:**
1. Go to [serpapi.com](https://serpapi.com)
2. Sign up for an account
3. Go to your dashboard and find the API key
4. Copy the key (long alphanumeric string)

**Free Tier:**
- 250 searches per month (free tier may change)
- See pricing and free-tier limits: https://serpapi.com/pricing
- Credit card might be required depending on plan

### 3. Alpha Vantage API Key (Required for fundamentals)

**Step-by-step:**
1. Go to [alphavantage.co/support/#api-key](https://www.alphavantage.co/support/#api-key)
2. Enter your email and click **GET FREE API KEY**
3. Key is sent to your email instantly
4. Copy the key (alphanumeric string)

**Free Tier:**
- 25 API requests per day
- 5 requests per minute
- No credit card required

### 4. Twelve Data API Key (Required for market data)

**Step-by-step:**
1. Go to [twelvedata.com](https://twelvedata.com)
2. Sign up for free account
3. Go to [Dashboard → API Key](https://twelvedata.com/apikey)
4. Copy your API key

**Free Tier:**
- Quota is based on credits per day (endpoint costs vary)
- See pricing and limits: https://twelvedata.com/pricing

---

## How It Works

### Development Mode
In development, the backend reads API keys from a `.env` file in the project root:

```bash
# Copy the example file
cp .env.example .env

# Edit with your keys
nano .env
```

The backend must be **restarted** after changing `.env`:
```bash
uv run finance_insight_api --host 0.0.0.0 --port 5000
```

### Production Mode
For production deployments, set environment variables directly:

**Docker (recommended):**
```bash
# Create .env with your secrets (keep it out of git)
docker run --env-file .env finance-insight
```

**Systemd Service:**
```ini
[Service]
Environment="OPENAI_API_KEY=sk-..."
Environment="SERPAPI_API_KEY=..."
Environment="TWELVE_DATA_API_KEY=..."
Environment="ALPHAVANTAGE_API_KEY=..."
```

**OpenChoreo / AMP:**
- Set these as environment variables when creating or updating the agent in the AMP UI.


## Required Keys

- **OPENAI_API_KEY** - AI agents won't work without this
- **SERPAPI_API_KEY** - Required for news search
- **TWELVE_DATA_API_KEY** - Required for market data
- **ALPHAVANTAGE_API_KEY** - Required for fundamentals
