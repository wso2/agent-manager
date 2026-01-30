import React from "react";

type HotelResult = {
  hotelId: string;
  hotelName: string;
  city: string;
  country: string;
  rating: number;
  lowestPrice: number;
  amenities: string[];
  mapUrl: string;
  imageUrl: string;
};

type HotelResultsPayload = {
  type: "hotel_search";
  summary: string;
  currency: string;
  hotels: HotelResult[];
};

type ChatHotelResultsProps = {
  payload: HotelResultsPayload;
};

const DEFAULT_CURRENCY = "USD";

const getHotelImageUrl = (hotel: HotelResult) => {
  if (hotel.imageUrl) {
    return hotel.imageUrl;
  }
  return `https://raw.githubusercontent.com/wso2con/2025-CMB-AI-tutorial/refs/heads/main/Lab-02-building-travel-planner/services/data/images/${hotel.hotelId}.jpeg`;
};

const clampRating = (rating: number) => {
  if (!Number.isFinite(rating)) {
    return 0;
  }
  return Math.min(5, Math.max(0, Math.round(rating)));
};

const formatPrice = (price: number, currency: string) => {
  if (!price || price <= 0) {
    return "N/A";
  }
  const code = currency || DEFAULT_CURRENCY;
  return `${code} ${price.toFixed(0)}`;
};

const extractLatLon = (mapUrl: string) => {
  const mlatMatch = mapUrl.match(/[?&]mlat=([^&]+)/);
  const mlonMatch = mapUrl.match(/[?&]mlon=([^&]+)/);
  if (mlatMatch && mlonMatch) {
    const lat = Number(mlatMatch[1]);
    const lon = Number(mlonMatch[1]);
    if (Number.isFinite(lat) && Number.isFinite(lon)) {
      return { lat, lon };
    }
  }
  const hashMatch = mapUrl.match(/#map=\d+\/([^/]+)\/([^/]+)/);
  if (hashMatch) {
    const lat = Number(hashMatch[1]);
    const lon = Number(hashMatch[2]);
    if (Number.isFinite(lat) && Number.isFinite(lon)) {
      return { lat, lon };
    }
  }
  return null;
};

const buildStaticMapUrl = (lat: number, lon: number) =>
  `https://staticmap.openstreetmap.de/staticmap.php?center=${lat},${lon}&zoom=15&size=360x200&markers=${lat},${lon},red-pushpin`;

export function ChatHotelResults({ payload }: ChatHotelResultsProps) {
  const currency = payload.currency || DEFAULT_CURRENCY;

  return (
    <div className="chat-hotel-results">
      {payload.summary && <p className="chat-hotels-summary">{payload.summary}</p>}
      <div className="hotels-grid">
        {payload.hotels.map((hotel) => {
          const ratingValue = clampRating(Number(hotel.rating || 0));
          const stars = "★".repeat(ratingValue) + "☆".repeat(5 - ratingValue);
          const location = [hotel.city, hotel.country].filter(Boolean).join(", ");
          const amenities = (hotel.amenities || []).slice(0, 3);
          const extraAmenities = (hotel.amenities || []).length - amenities.length;
          const latLon = hotel.mapUrl ? extractLatLon(hotel.mapUrl) : null;
          const mapImageUrl = latLon ? buildStaticMapUrl(latLon.lat, latLon.lon) : "";
          return (
            <div key={hotel.hotelId} className="hotel-card">
              <div className="hotel-image">
                <img
                  src={getHotelImageUrl(hotel)}
                  alt={hotel.hotelName || "Hotel"}
                  loading="lazy"
                  onError={(event) => {
                    const target = event.currentTarget;
                    target.src =
                      "data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='800' height='480' viewBox='0 0 800 480'><rect width='800' height='480' fill='%23e8f6f5'/><text x='50%25' y='50%25' text-anchor='middle' font-family='Inter, Arial, sans-serif' font-size='24' fill='%235a6c7d'>Hotel Image</text></svg>";
                  }}
                />
                <div className="hotel-image-overlay">
                  <span className="hotel-id">ID: {hotel.hotelId || "N/A"}</span>
                </div>
                <div className="hotel-price-tag">
                  <span>{formatPrice(Number(hotel.lowestPrice || 0), currency)}</span>
                </div>
              </div>

              <div className="hotel-content">
                <div className="hotel-header">
                  <h4>{hotel.hotelName || "Hotel"}</h4>
                  <div className="hotel-rating">
                    <span className="rating-stars">{stars}</span>
                    <span className="rating-text">{ratingValue.toFixed(1)}</span>
                  </div>
                </div>

                <p className="hotel-location">
                  <span className="location-icon">📍</span> {location || "Location unavailable"}
                </p>
                {hotel.mapUrl && (
                  <a
                    className="hotel-map-link"
                    href={hotel.mapUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    View on map
                  </a>
                )}
                {hotel.mapUrl && mapImageUrl && (
                  <a
                    className="hotel-map-preview"
                    href={hotel.mapUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <img src={mapImageUrl} alt={`${hotel.hotelName} map`} loading="lazy" />
                  </a>
                )}

                <div className="hotel-features">
                  <div className="amenities-list">
                    {amenities.map((amenity) => (
                      <span key={amenity} className="amenity-tag">
                        {amenity}
                      </span>
                    ))}
                    {extraAmenities > 0 && (
                      <span className="amenity-more">+{extraAmenities}</span>
                    )}
                  </div>
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
