import { Compass } from "lucide-react";

export function Footer() {
  return (
    <footer className="bg-primary text-primary-foreground py-12 md:py-16 mt-20">
      <div className="container mx-auto px-4">
        <div className="grid grid-cols-1 gap-12">
          <div className="space-y-4">
            <div className="flex items-center gap-2">
              <div className="bg-accent text-accent-foreground p-1 rounded-md">
                <Compass className="w-5 h-5" />
              </div>
              <span className="font-display font-bold text-xl tracking-tight text-white">
                Hotel Booking Agent
              </span>
            </div>
            <p className="text-primary-foreground/70 text-sm leading-relaxed max-w-xs">
              AI-powered travel planning that helps you discover the world's most beautiful destinations tailored just for you.
            </p>
          </div>
        </div>
        
        <div className="border-t border-white/10 mt-12 pt-8 text-center text-xs text-primary-foreground/50">
          © {new Date().getFullYear()} Hotel Booking Agent Inc. All rights reserved.
        </div>
      </div>
    </footer>
  );
}
