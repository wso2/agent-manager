import { Link } from "wouter";
import { useEffect, useState } from "react";
import { Navbar } from "components/Navbar";
import { Button } from "components/ui/button";
import { Calendar, CheckCircle2, User } from "lucide-react";
import { useAsgardeo } from "@asgardeo/react";

type Booking = {
  bookingId: string;
  hotelName?: string;
  hotelId?: string;
  bookingStatus?: string;
  bookingDate?: string;
  checkInDate?: string;
  checkOutDate?: string;
  numberOfGuests?: number;
  numberOfRooms?: number;
  rooms?: Array<{
    roomId?: string;
    numberOfRooms?: number;
  }>;
  roomType?: string;
  provider?: string;
  confirmationNumber?: string;
  pricing?: Array<{
    roomRate?: number;
    totalAmount?: number;
    nights?: number;
    currency?: string;
  }>;
};

const API_BASE_URL =
  process.env.REACT_APP_API_BASE_URL || "http://localhost:9090";
const USER_ID_STORAGE_KEY = "travelPlannerUserId";

const createSessionId = () =>
  `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;

const getOrCreateUserId = () => {
  if (typeof window === "undefined") {
    return "default";
  }
  const existing = localStorage.getItem(USER_ID_STORAGE_KEY);
  if (existing) {
    return existing;
  }
  const newId =
    typeof crypto !== "undefined" && "randomUUID" in crypto
      ? crypto.randomUUID()
      : createSessionId();
  localStorage.setItem(USER_ID_STORAGE_KEY, newId);
  return newId;
};

const formatDate = (value?: string) => {
  if (!value) {
    return "TBD";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleDateString();
};

const formatCurrency = (value?: number, currency?: string) => {
  if (value === undefined || value === null) {
    return null;
  }
  const resolved = currency || "USD";
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: resolved,
      maximumFractionDigits: 0,
    }).format(value);
  } catch {
    return `${value} ${resolved}`;
  }
};

export default function BookingSummary() {
  const { getAccessToken, isSignedIn } = useAsgardeo();
  const [bookings, setBookings] = useState<Booking[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const loadBookings = async () => {
      try {
        if (!isSignedIn) {
          setBookings([]);
          setLoading(false);
          return;
        }
        const userId = getOrCreateUserId();
        const token = await getAccessToken();
        const headers: Record<string, string> = { "x-user-id": userId };
        if (token) {
          headers.Authorization = `Bearer ${token}`;
        }
        const response = await fetch(`${API_BASE_URL}/bookings`, { headers });
        if (!response.ok) {
          throw new Error(`Failed to load bookings: ${response.status}`);
        }
        const data = await response.json();
        if (Array.isArray(data)) {
          setBookings(data);
        } else {
          setBookings([]);
        }
      } catch (err) {
        if ((err as Error)?.name !== "AbortError") {
          setError("Unable to load bookings right now.");
        }
      } finally {
        setLoading(false);
      }
    };
    loadBookings();
  }, [getAccessToken, isSignedIn]);

  return (
    <div className="min-h-screen flex flex-col bg-[linear-gradient(180deg,#d4e3ff_0%,#e4ecf7_55%,#f8fafc_100%)]">
      <Navbar variant="themed" />

      <main className="flex-grow container mx-auto px-4 py-16">
        <div className="max-w-4xl mx-auto space-y-10">
          <div className="bg-white/90 rounded-3xl border border-white/60 shadow-xl p-8 backdrop-blur-sm">
            <div className="flex items-center gap-4">
              <div className="w-14 h-14 rounded-full bg-[#0f4c9f]/10 text-[#0f4c9f] flex items-center justify-center">
                <CheckCircle2 className="w-7 h-7" />
              </div>
              <div>
                <h1 className="text-3xl font-semibold text-slate-900">My Bookings</h1>
                <p className="text-slate-500">
                  {loading ? "Loading your bookings..." : `${bookings.length} bookings`}
                </p>
              </div>
            </div>
          </div>

          <div className="bg-white/90 rounded-3xl border border-white/60 shadow-xl p-8 space-y-6 backdrop-blur-sm">
            {loading && (
              <p className="text-slate-500">Loading bookings...</p>
            )}
            {!loading && error && (
              <p className="text-destructive">{error}</p>
            )}
            {!loading && !error && bookings.length === 0 && (
              <p className="text-slate-500">0 bookings.</p>
            )}
            {!loading && !error && bookings.length > 0 && (
              <div className="space-y-4">
                {bookings.map((booking) => {
                  const pricing = booking.pricing?.[0];
                  const priceText = pricing
                    ? formatCurrency(pricing.totalAmount, pricing.currency)
                    : null;
                  const nightsText = pricing?.nights
                    ? `${pricing.nights} nights`
                    : null;
                  return (
                    <div
                      key={booking.bookingId}
                      className="border border-white/70 rounded-2xl p-6 flex flex-col md:flex-row md:items-center md:justify-between gap-6 bg-white/70"
                    >
                      <div className="space-y-2">
                        <p className="text-sm text-slate-500">Booking ID</p>
                        <h3 className="text-xl font-semibold text-slate-900">
                          {booking.hotelName || booking.hotelId || "Hotel stay"}
                        </h3>
                        <p className="text-sm text-slate-500">
                          {booking.bookingId}
                        </p>
                      </div>
                      <div className="space-y-2 text-sm text-slate-500">
                        <div className="flex items-center gap-2">
                          <Calendar className="w-4 h-4 text-[#0f4c9f]" />
                          <span>
                            {formatDate(booking.checkInDate)} - {formatDate(booking.checkOutDate)}
                          </span>
                        </div>
                        <div className="flex items-center gap-2">
                          <User className="w-4 h-4 text-[#0f4c9f]" />
                          <span>{booking.numberOfGuests || 0} guests</span>
                        </div>
                        <p>
                          Rooms:{" "}
                          <span className="font-semibold text-slate-900">
                            {booking.numberOfRooms || booking.rooms?.length || 0}
                          </span>
                        </p>
                        <p>
                          Status:{" "}
                          <span className="font-semibold text-slate-900">
                            {booking.bookingStatus || "PENDING"}
                          </span>
                        </p>
                        {booking.roomType && (
                          <p className="text-slate-500">
                            Room:{" "}
                            <span className="font-semibold text-slate-900">
                              {booking.roomType}
                            </span>
                          </p>
                        )}
                        {!booking.roomType && booking.provider && (
                          <p className="text-slate-500">
                            Provider:{" "}
                            <span className="font-semibold text-slate-900">
                              {booking.provider}
                            </span>
                          </p>
                        )}
                      </div>
                      <div className="text-right space-y-2">
                        {priceText && (
                          <p className="text-lg font-semibold text-[#0f4c9f]">
                            {priceText}
                          </p>
                        )}
                        {nightsText && (
                          <p className="text-sm text-slate-500">{nightsText}</p>
                        )}
                        {booking.confirmationNumber && (
                          <p className="text-xs text-slate-500">
                            Confirmation {booking.confirmationNumber}
                          </p>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
            <div className="flex flex-wrap gap-3 pt-4">
              <Button asChild className="rounded-full bg-[#0f4c9f] hover:bg-[#0d4490] text-white">
                <Link href="/assistant">Back to chat</Link>
              </Button>
              <Button
                asChild
                variant="outline"
                className="rounded-full border-[#0f4c9f]/30 text-[#0f4c9f] hover:bg-[#d4e3ff]"
              >
                <Link href="/">Explore stays</Link>
              </Button>
            </div>
          </div>
        </div>
      </main>
    </div>
  );
}
