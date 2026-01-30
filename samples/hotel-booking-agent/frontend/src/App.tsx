import { Switch, Route, useLocation } from "wouter";
import { useAsgardeo } from "@asgardeo/react";
import { queryClient } from "./lib/queryClient";
import { QueryClientProvider } from "@tanstack/react-query";
import { Toaster } from "components/ui/toaster";
import { TooltipProvider } from "components/ui/tooltip";
import NotFound from "pages/not-found";
import Home from "pages/Home";
import Landing from "pages/Landing";
import SignIn from "pages/SignIn";

function Router() {
  const { isSignedIn, isLoading } = useAsgardeo();
  const [location] = useLocation();
  const requiresAuth =
    location.startsWith("/assistant");

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <p className="text-muted-foreground">Loading your session...</p>
      </div>
    );
  }

  if (!isSignedIn && requiresAuth) {
    return <SignIn />;
  }

  return (
    <Switch>
      <Route path="/" component={Landing} />
      <Route path="/assistant" component={Home} />
      <Route path="/signin" component={SignIn} />
      <Route component={NotFound} />
    </Switch>
  );
}

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <Toaster />
        <Router />
      </TooltipProvider>
    </QueryClientProvider>
  );
}

export default App;
