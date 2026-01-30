import { SignInButton, SignUpButton } from "@asgardeo/react";
import { Compass } from "lucide-react";

export default function SignIn() {
  return (
    <div className="min-h-screen flex items-center justify-center bg-background px-6">
      <div className="w-full max-w-md rounded-3xl border border-border/60 bg-white p-10 shadow-2xl">
        <div className="flex items-center gap-3 text-primary">
          <div className="rounded-xl bg-primary/10 p-3">
            <Compass className="h-6 w-6" />
          </div>
          <div>
            <h1 className="font-display text-2xl font-bold text-foreground">
              Hotel Booking Agent
            </h1>
            <p className="text-sm text-muted-foreground">
              Sign in to unlock your AI trip assistant
            </p>
          </div>
        </div>

        <div className="mt-10 space-y-4">
          <SignInButton className="w-full rounded-2xl bg-primary py-3 text-base font-semibold text-white shadow-lg transition hover:bg-primary/90">
            Sign In with Asgardeo
          </SignInButton>
          <SignUpButton className="w-full rounded-2xl border border-border py-3 text-base font-semibold text-foreground shadow-sm transition hover:bg-muted/40">
            Create an Account
          </SignUpButton>
          <p className="text-xs text-muted-foreground">
            We use Asgardeo to keep your account secure.
          </p>
        </div>
      </div>
    </div>
  );
}
