"use client";

import { useEffect, useRef, useState } from "react";
import Icon from "./Icon";

type Theme = "light" | "dark";

const STORAGE_KEY = "theme";

const applyTheme = (theme: Theme, animate = false) => {
  const root = document.documentElement;
  if (animate) {
    root.classList.remove("theme-transition");
    void root.offsetHeight;
    root.classList.add("theme-transition");
    window.setTimeout(() => {
      root.classList.remove("theme-transition");
    }, 350);
  }
  root.dataset.theme = theme;
  root.style.colorScheme = theme;
};

const getSystemTheme = (): Theme =>
  window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";

export default function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>("light");
  const transitionRef = useRef<number | null>(null);

  useEffect(() => {
    let stored: string | null = null;
    try {
      stored = window.localStorage.getItem(STORAGE_KEY);
    } catch (error) {
      console.warn("[Theme] localStorage unavailable:", error);
    }
    const initial =
      stored === "light" || stored === "dark" ? stored : getSystemTheme();
    setTheme(initial);
    applyTheme(initial);
  }, []);

  const toggleTheme = () => {
    const next = theme === "dark" ? "light" : "dark";
    setTheme(next);
    try {
      window.localStorage.setItem(STORAGE_KEY, next);
    } catch (error) {
      console.warn("[Theme] localStorage unavailable:", error);
    }
    if (transitionRef.current) {
      window.clearTimeout(transitionRef.current);
    }
    applyTheme(next, true);
    transitionRef.current = window.setTimeout(() => {
      transitionRef.current = null;
    }, 350);
  };

  return (
    <button className="theme-toggle" onClick={toggleTheme} type="button">
      <Icon name={theme === "dark" ? "sun" : "moon"} />
      <span>{theme === "dark" ? "Light mode" : "Dark mode"}</span>
    </button>
  );
}
