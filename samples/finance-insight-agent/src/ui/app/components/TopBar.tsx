import Link from "next/link";
import ThemeToggle from "./ThemeToggle";

type TopBarProps = {
  onNewChat: () => void;
};

export default function TopBar({ onNewChat }: TopBarProps) {
  return (
    <header className="topbar">
      <div className="topbar-left">
        <img
          src="/images/WSO2_Software_Logo.png"
          alt="WSO2"
          className="brand-logo"
        />
        <div className="brand">Finance Insight</div>
      </div>
      <div className="topbar-right">
        <button className="button-secondary" onClick={onNewChat} type="button">
          New request
        </button>
        <Link className="button-secondary" href="/settings">
          Settings
        </Link>
        <ThemeToggle />
      </div>
    </header>
  );
}
