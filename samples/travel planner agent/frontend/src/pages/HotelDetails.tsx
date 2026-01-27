import { useRoute, Link } from "wouter";
import { useHotel } from "hooks/use-hotels";
import { Navbar } from "components/Navbar";
import { Footer } from "components/Footer";
import { Badge } from "components/ui/badge";
import { Button } from "components/ui/button";
import { Skeleton } from "components/ui/skeleton";
import { Star, MapPin, Wifi, Utensils, Waves, Check, ShieldCheck, ChevronLeft, Globe, Mail, ExternalLink } from "lucide-react";
import { motion } from "framer-motion";

const FALLBACK_IMAGE =
  "data:image/svg+xml;utf8," +
  "<svg xmlns='http://www.w3.org/2000/svg' width='1200' height='700' viewBox='0 0 1200 700'>" +
  "<defs><linearGradient id='g' x1='0' x2='1' y1='0' y2='1'>" +
  "<stop stop-color='%230b1f24' offset='0'/>" +
  "<stop stop-color='%23194e56' offset='1'/>" +
  "</linearGradient></defs>" +
  "<rect width='1200' height='700' fill='url(%23g)'/>" +
  "<circle cx='900' cy='160' r='140' fill='%23ffffff' opacity='0.08'/>" +
  "<circle cx='260' cy='560' r='180' fill='%23ffffff' opacity='0.05'/>" +
  "<text x='50%' y='50%' text-anchor='middle' font-family='Inter, Arial, sans-serif' font-size='36' fill='%23ffffff' opacity='0.6'>Hotel Preview</text>" +
  "</svg>";

export default function HotelDetails() {
  const [match, params] = useRoute<{ id: string }>("/hotels/:id");
  const id = params ? parseInt(params.id) : 0;
  
  const { data: hotel, isLoading: isLoadingHotel } = useHotel(id);

  if (isLoadingHotel || !hotel) {
    return (
      <div className="min-h-screen flex flex-col">
        <Navbar />
        <div className="container mx-auto px-4 py-8 flex-grow space-y-8">
          <Skeleton className="h-[400px] w-full rounded-3xl" />
          <div className="space-y-4">
            <Skeleton className="h-10 w-2/3" />
            <Skeleton className="h-6 w-1/3" />
            <Skeleton className="h-32 w-full" />
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen flex flex-col bg-background">
      <Navbar />
      
      <main className="flex-grow">
        {/* Hero Image Section */}
        <div className="relative h-[50vh] min-h-[400px] w-full overflow-hidden">
          <div className="absolute inset-0 bg-gradient-to-t from-black/60 to-transparent z-10" />
          <img 
            src={hotel.imageUrl} 
            alt={hotel.name}
            loading="lazy"
            onError={(event) => {
              event.currentTarget.src = FALLBACK_IMAGE;
            }}
            className="w-full h-full object-cover"
          />
          <div className="absolute bottom-0 left-0 w-full z-20 p-6 md:p-12 text-white container mx-auto">
            <Link href="/assistant" className="inline-flex items-center gap-2 text-white/80 hover:text-white mb-6 transition-colors font-medium">
              <ChevronLeft className="w-4 h-4" /> Back to planning
            </Link>
            <motion.div
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.5 }}
            >
              <div className="flex flex-wrap items-center gap-3 mb-3">
                <Badge className="bg-accent text-accent-foreground border-none px-3 py-1 text-sm font-semibold">
                  {hotel.tags[0]}
                </Badge>
                <div className="flex items-center gap-1 bg-black/40 backdrop-blur-md px-3 py-1 rounded-full text-sm font-medium">
                  <Star className="w-4 h-4 text-yellow-400 fill-yellow-400" />
                  {hotel.rating} ({hotel.reviewCount} reviews)
                </div>
              </div>
              <h1 className="font-display font-bold text-4xl md:text-6xl mb-2">{hotel.name}</h1>
              <div className="flex items-center gap-2 text-lg text-white/90">
                <MapPin className="w-5 h-5" />
                {hotel.location}
              </div>
            </motion.div>
          </div>
        </div>

        <div className="container mx-auto px-4 py-12 grid grid-cols-1 lg:grid-cols-3 gap-12">
          {/* Left Column: Details */}
          <div className="lg:col-span-2 space-y-12">
            <section>
              <h2 className="font-display text-2xl font-bold mb-4 text-primary">About this stay</h2>
              <p className="text-muted-foreground leading-relaxed text-lg">
                {hotel.description}
              </p>
            </section>

            <section>
              <h2 className="font-display text-2xl font-bold mb-6 text-primary">Popular Amenities</h2>
              <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
                {hotel.amenities.map((amenity, idx) => (
                  <div key={idx} className="flex items-center gap-3 p-3 rounded-xl border border-border/50 bg-secondary/20">
                    <div className="p-2 bg-white rounded-full text-primary shadow-sm">
                      {amenity === "Pool" && <Waves className="w-4 h-4" />}
                      {amenity === "Restaurant" && <Utensils className="w-4 h-4" />}
                      {amenity === "Free WiFi" && <Wifi className="w-4 h-4" />}
                      {!["Pool", "Restaurant", "Free WiFi"].includes(amenity) && <ShieldCheck className="w-4 h-4" />}
                    </div>
                    <span className="font-medium">{amenity}</span>
                  </div>
                ))}
              </div>
            </section>

            <section>
              <h2 className="font-display text-2xl font-bold mb-6 text-primary">Location</h2>
              <div className="rounded-2xl overflow-hidden border border-border h-[400px] relative bg-muted flex items-center justify-center">
                {/* Mock Map UI */}
                <div className="absolute inset-0 z-0">
                  <img 
                    src={`https://api.mapbox.com/styles/v1/mapbox/light-v10/static/pin-s-l+285A98(${hotel.longitude},${hotel.latitude})/${hotel.longitude},${hotel.latitude},12/800x400?access_token=pk.eyJ1IjoicmVwbGl0IiwiYSI6ImNreHg0Z2o0ZTBmMXAydm56Nmx6Nmx6bnAifQ.x-x-x`}
                    alt="Map location"
                    className="w-full h-full object-cover opacity-50 grayscale"
                  />
                </div>
                <div className="relative z-10 bg-white/90 backdrop-blur-md p-6 rounded-2xl shadow-xl border border-border/50 max-w-sm text-center">
                  <MapPin className="w-8 h-8 text-primary mx-auto mb-3" />
                  <h3 className="font-bold text-lg mb-1">{hotel.name}</h3>
                  <p className="text-sm text-muted-foreground mb-4">{hotel.location}</p>
                  <Button variant="outline" className="w-full rounded-xl gap-2" asChild>
                    <a href={`https://www.google.com/maps/search/?api=1&query=${hotel.latitude},${hotel.longitude}`} target="_blank" rel="noopener noreferrer">
                      <ExternalLink className="w-4 h-4" /> Open in Google Maps
                    </a>
                  </Button>
                </div>
              </div>
            </section>
          </div>

          {/* Right Column: Contact & Info */}
          <div className="lg:col-span-1">
            <div className="sticky top-24 rounded-2xl border border-border bg-white shadow-xl p-6 space-y-8">
              <div className="space-y-4">
                <h3 className="font-display text-xl font-bold text-primary">Contact Details</h3>
                
                <div className="space-y-3">
                  {hotel.website && (
                    <Button variant="outline" className="w-full justify-start gap-3 h-12 rounded-xl border-border hover:bg-muted/50 transition-colors" asChild>
                      <a href={hotel.website} target="_blank" rel="noopener noreferrer">
                        <Globe className="w-5 h-5 text-primary" />
                        <span className="truncate">{hotel.website.replace('https://', '')}</span>
                      </a>
                    </Button>
                  )}
                  
                  {hotel.email && (
                    <Button variant="outline" className="w-full justify-start gap-3 h-12 rounded-xl border-border hover:bg-muted/50 transition-colors" asChild>
                      <a href={`mailto:${hotel.email}`}>
                        <Mail className="w-5 h-5 text-primary" />
                        <span className="truncate">{hotel.email}</span>
                      </a>
                    </Button>
                  )}
                </div>
              </div>

              <div className="p-5 bg-primary/5 rounded-2xl border border-primary/10 space-y-4">
                <h4 className="font-bold text-primary flex items-center gap-2">
                  <ShieldCheck className="w-5 h-5" />
                  Booking via Assistant
                </h4>
                <p className="text-sm text-muted-foreground leading-relaxed">
                  Our AI agent handles the entire booking process for you. No need to select rooms manually—just tell the agent your dates and preferences in the chat.
                </p>
                <Button 
                  className="w-full bg-primary hover:bg-primary/90 text-white rounded-xl shadow-lg shadow-primary/20"
                  asChild
                >
                  <Link href="/assistant">Return to Chat</Link>
                </Button>
              </div>
            </div>
          </div>
        </div>
      </main>

      <Footer />
    </div>
  );
}
