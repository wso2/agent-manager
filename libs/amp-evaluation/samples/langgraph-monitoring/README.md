# README.md

## Customer Support Agent — Evaluation Monitor

Monitors the quality of the Swiss Airlines customer support agent by
evaluating traces against 5 evaluators.

### Evaluators

| Evaluator | What it checks | Tags |
|-----------|---------------|------|
| tool-call-relevance | Are tools from the right domain? | tool-use, quality |
| response-grounding | Are specific claims backed by tool results? | hallucination, grounding |
| tool-success-rate | Did tools execute without errors? | tool-use, reliability |
| response-completeness | Is the response complete and not broken? | quality, output |
| llm-hallucination-judge | LLM-verified hallucination detection | hallucination, llm-judge |

### Quick Start

```bash
# Install dependencies
pip install -r requirements.txt

# Set API keys
export AMP_API_KEY="your-amp-api-key"
export OPENAI_API_KEY="your-openai-key"   # optional, for LLM judge

# Run with defaults (last 7 days, 100 traces)
python run_monitor.py

# Quick test with 5 traces
python run_monitor.py --limit 5

# Specific time range
python run_monitor.py --start 2025-07-01T00:00:00Z --end 2025-07-08T00:00:00Z

# Production environment
python run_monitor.py --environment Production --limit 500