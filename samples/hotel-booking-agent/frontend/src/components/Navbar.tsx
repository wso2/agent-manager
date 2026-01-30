import { Link, useLocation } from "wouter";
import { Compass, Globe, LogOut, Menu } from "lucide-react";
import { Button } from "components/ui/button";
import { SignedIn, SignedOut, SignInButton, SignOutButton, useAsgardeo } from "@asgardeo/react";

type NavbarVariant = "default" | "themed";

interface NavbarProps {
  variant?: NavbarVariant;
}

export function Navbar({ variant = "default" }: NavbarProps) {
  const [location] = useLocation();
  const { isLoading, user } = useAsgardeo();
  const displayName = (() => {
    const raw =
      user?.displayName ||
      user?.given_name ||
      user?.preferred_username ||
      user?.username ||
      user?.email ||
      null;
    if (!raw) {
      return null;
    }
    if (raw.includes("@")) {
      return raw.split("@")[0];
    }
    return raw;
  })();
  const isThemed = variant === "themed";

  return (
    <nav
      className={
        isThemed
          ? "sticky top-0 z-50 w-full bg-gradient-to-r from-[#0f4c9f] to-[#0a3c82] text-white shadow-lg"
          : "sticky top-0 z-50 w-full border-b border-border/40 bg-white/80 backdrop-blur-md"
      }
    >
      <div className="container mx-auto px-4 h-16 flex items-center">
        <Link
          href="/"
          className={
            isThemed
              ? "flex items-center gap-3 rounded-full border border-white/25 bg-white/15 px-3 py-2 text-white shadow-sm transition-colors hover:bg-white/20"
              : "flex items-center gap-2 group"
          }
        >
          <div
            className={
              isThemed
                ? "flex h-8 w-8 items-center justify-center rounded-lg bg-white text-[#0f4c9f]"
                : "bg-primary text-white p-1.5 rounded-lg group-hover:bg-primary/90 transition-colors"
            }
          >
            {isThemed ? <Globe className="w-4 h-4" /> : <Compass className="w-6 h-6" />}
          </div>
          <span
            className={
              isThemed
                ? "text-lg font-semibold text-white tracking-tight"
                : "font-display font-bold text-xl text-primary tracking-tight"
            }
          >
            Hotel Booking Agent
          </span>
        </Link>

        <div className="flex-1 hidden md:flex items-center justify-center">
          <Link
            href="/"
            className={`text-sm font-medium transition-colors ${
              isThemed ? "hover:text-white" : "hover:text-primary"
            } ${location === "/" ? (isThemed ? "text-white" : "text-primary") : isThemed ? "text-white/80" : "text-muted-foreground"}`}
          >
            Explore
          </Link>
        </div>

        <div className="flex items-center gap-3 ml-auto">
          <SignedOut>
            <SignInButton
              className={
                isThemed
                  ? "rounded-xl bg-white/15 px-4 py-2 text-sm font-semibold text-white hover:bg-white/25"
                  : "rounded-xl bg-primary px-4 py-2 text-sm font-semibold text-white hover:bg-primary/90"
              }
            >
              Sign In
            </SignInButton>
          </SignedOut>
          <SignedIn>
            <div className="flex items-center gap-4">
              <div
                className={`flex items-center gap-4 rounded-full px-4 py-2 ${
                  isThemed ? "border border-white/20 bg-white/10" : "border border-white/20 bg-white/10 backdrop-blur-md"
                }`}
              >
                <div
                  className={
                    isThemed
                      ? "flex h-10 w-10 items-center justify-center rounded-full bg-white/20 text-base font-semibold text-white shadow-sm"
                      : "flex h-10 w-10 items-center justify-center rounded-full bg-primary text-base font-semibold text-white shadow-sm"
                  }
                >
                  {(displayName || "U").charAt(0).toUpperCase()}
                </div>
                <div className="hidden sm:flex flex-col items-start">
                  <span className={isThemed ? "text-xs text-white/70" : "text-xs text-muted-foreground"}>Welcome back</span>
                  <span className={isThemed ? "text-sm font-semibold text-white" : "text-sm font-semibold text-foreground"}>
                    {isLoading ? "..." : displayName || ""}
                  </span>
                </div>
              </div>
              <SignOutButton
                className={
                  isThemed
                    ? "flex items-center gap-2 rounded-full border border-white/20 bg-white/10 px-4 py-2 text-sm font-medium text-white transition hover:-translate-y-0.5 hover:bg-white/20"
                    : "flex items-center gap-2 rounded-full border border-border bg-white/10 px-4 py-2 text-sm font-medium text-foreground transition hover:-translate-y-0.5 hover:bg-muted/40"
                }
              >
                <LogOut className="h-4 w-4" />
                <span>Sign Out</span>
              </SignOutButton>
            </div>
          </SignedIn>
          <Button variant="ghost" size="icon" className="md:hidden">
            <Menu className="w-5 h-5" />
          </Button>
        </div>
      </div>
    </nav>
  );
}
