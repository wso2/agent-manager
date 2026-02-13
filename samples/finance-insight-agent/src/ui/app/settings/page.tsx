"use client";

import Link from "next/link";
import SettingsForm from "../components/SettingsForm";
import ThemeToggle from "../components/ThemeToggle";

export default function SettingsPage() {
  return (
    <div className="settings-page">
      <header className="settings-topbar">
        <div className="settings-brand">
          <img
            src="/images/WSO2_Software_Logo.png"
            alt="WSO2"
            className="brand-logo"
          />
          <span>Finance Insight</span>
        </div>
        <div className="settings-actions">
          <Link className="button-secondary" href="/">
            Back to chat
          </Link>
          <ThemeToggle />
        </div>
      </header>
      <div className="settings-container">
        <SettingsForm />
      </div>
    </div>
  );
}
