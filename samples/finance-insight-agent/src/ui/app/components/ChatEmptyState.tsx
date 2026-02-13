"use client";

type Scenario = {
  title: string;
  description: string;
  prompt: string;
};

const SCENARIOS: Scenario[] = [
  {
    title: "Company Deep Dive",
    description: "News drivers, risks, and key metrics for a single stock.",
    prompt: "Give me a deep dive on NVIDIA with recent drivers and key metrics.",
  },
  {
    title: "Earnings Reaction",
    description: "Summarize an earnings event and quantify the market move.",
    prompt: "Summarize Apple earnings and quantify the latest price reaction.",
  },
  {
    title: "Macro Pulse",
    description: "Track macro events and their market impact.",
    prompt: "How are rate expectations affecting bank stocks this week?",
  },
];

type ChatEmptyStateProps = {
  onSelectScenario?: (prompt: string) => void;
};

export default function ChatEmptyState({ onSelectScenario }: ChatEmptyStateProps) {
  return (
    <div className="empty-state">
      <h1>Welcome to Finance Insight</h1>
      <p className="empty-state-subtitle">
        Submit a request and we will generate a report when it is ready.
      </p>
      <div className="scenario-grid">
        {SCENARIOS.map((scenario) => (
          <button
            key={scenario.title}
            type="button"
            className="scenario-card"
            onClick={() => onSelectScenario?.(scenario.prompt)}
          >
            <span className="scenario-title">{scenario.title}</span>
            <span className="scenario-description">{scenario.description}</span>
            <span className="scenario-prompt">Try: "{scenario.prompt}"</span>
          </button>
        ))}
      </div>
    </div>
  );
}
