import { Link } from "wouter";
import { Star, MapPin, ArrowRight } from "lucide-react";
import { type Hotel } from "shared/schema";
import { Badge } from "components/ui/badge";

interface HotelCardProps {
  hotel: Hotel;
}

const FALLBACK_IMAGE =
  "data:image/svg+xml;utf8," +
  "<svg xmlns='http://www.w3.org/2000/svg' width='800' height='480' viewBox='0 0 800 480'>" +
  "<defs><linearGradient id='g' x1='0' x2='1' y1='0' y2='1'>" +
  "<stop stop-color='%23e8f6f5' offset='0'/>" +
  "<stop stop-color='%23bfeee7' offset='1'/>" +
  "</linearGradient></defs>" +
  "<rect width='800' height='480' fill='url(%23g)'/>" +
  "<circle cx='620' cy='120' r='90' fill='%230b3a3a' opacity='0.08'/>" +
  "<circle cx='180' cy='360' r='140' fill='%230b3a3a' opacity='0.05'/>" +
  "<text x='50%' y='52%' text-anchor='middle' font-family='Inter, Arial, sans-serif' font-size='28' fill='%230b3a3a' opacity='0.5'>Hotel Preview</text>" +
  "</svg>";

export function HotelCard({ hotel }: HotelCardProps) {
  const handleImageError = (event: React.SyntheticEvent<HTMLImageElement>) => {
    event.currentTarget.src = FALLBACK_IMAGE;
  };

  return (
    <Link href={`/hotels/${hotel.id}`} className="group block h-full">
      <div className="bg-card rounded-2xl overflow-hidden border border-border/50 shadow-sm hover:shadow-xl hover:shadow-primary/5 hover:-translate-y-1 transition-all duration-300 h-full flex flex-col">
        {/* Image Container */}
        <div className="relative h-56 overflow-hidden">
          <img 
            src={hotel.imageUrl} 
            alt={hotel.name}
            loading="lazy"
            onError={handleImageError}
            className="w-full h-full object-cover transition-transform duration-700 group-hover:scale-110"
          />
          <div className="absolute top-4 right-4 bg-white/90 backdrop-blur-sm px-2 py-1 rounded-lg flex items-center gap-1 shadow-sm">
            <Star className="w-3.5 h-3.5 fill-yellow-400 text-yellow-400" />
            <span className="text-xs font-bold text-primary">{hotel.rating}</span>
          </div>
          {hotel.tags[0] && (
            <Badge className="absolute bottom-4 left-4 bg-accent text-accent-foreground hover:bg-accent/90 border-none shadow-sm">
              {hotel.tags[0]}
            </Badge>
          )}
        </div>

        {/* Content */}
        <div className="p-5 flex flex-col flex-grow">
          <div className="flex items-start justify-between gap-2 mb-2">
            <h3 className="font-display font-bold text-lg leading-tight text-primary group-hover:text-primary/80 transition-colors">
              {hotel.name}
            </h3>
          </div>
          
          <div className="flex items-center text-muted-foreground mb-4">
            <MapPin className="w-3.5 h-3.5 mr-1" />
            <span className="text-xs">{hotel.location}</span>
          </div>
          
          <p className="text-sm text-muted-foreground line-clamp-2 mb-4 flex-grow">
            {hotel.description}
          </p>

          <div className="pt-4 border-t border-border/50 flex items-center justify-between mt-auto">
            <div>
              <span className="text-xs text-muted-foreground uppercase tracking-wider font-semibold">From</span>
              <div className="flex items-baseline gap-1">
                <span className="text-lg font-bold text-primary">${hotel.pricePerNight}</span>
                <span className="text-xs text-muted-foreground">/night</span>
              </div>
            </div>
            
            <div className="w-8 h-8 rounded-full bg-primary/5 flex items-center justify-center group-hover:bg-primary group-hover:text-white transition-colors duration-300">
              <ArrowRight className="w-4 h-4" />
            </div>
          </div>
        </div>
      </div>
    </Link>
  );
}
