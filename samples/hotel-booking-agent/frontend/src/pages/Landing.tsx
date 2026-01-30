import { Navbar } from "components/Navbar";
import { motion } from "framer-motion";
import { Button } from "components/ui/button";
import { useLocation } from "wouter";
import { ArrowRight } from "lucide-react";

export default function Landing() {
  const [, setLocation] = useLocation();

  return (
    <div className="h-screen flex flex-col bg-background overflow-hidden">
      <Navbar variant="themed" />
      
      <main className="flex-grow">
        <section
          className="h-full flex items-center justify-center bg-cover bg-center"
          style={{ backgroundImage: "url('/images/orchid-hotels.webp')" }}
        >
          <div className="absolute inset-0 bg-gradient-to-r from-transparent via-[#0f4c9f]/25 to-[#0f4c9f]/55" />
          <div className="text-left px-4 max-w-3xl space-y-6 -translate-y-6">
            <motion.div
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.6 }}
            >
              <p className="text-3xl md:text-5xl font-bold text-white drop-shadow-[0_6px_18px_rgba(15,23,42,0.35)] relative">
                Search the best stays
                <br />
                Book it instantly
              </p>
              <Button
                size="lg"
                onClick={() => setLocation("/assistant")}
                className="mt-10 bg-[#ffb347] hover:bg-[#ff9f1c] text-slate-900 font-semibold text-base md:text-lg px-8 py-4 rounded-full h-auto shadow-lg relative"
              >
                Start Planning Now
                <ArrowRight className="ml-2 w-5 h-5 group-hover:translate-x-1 transition-transform" />
              </Button>
            </motion.div>
          </div>
        </section>
      </main>
    </div>
  );
}
