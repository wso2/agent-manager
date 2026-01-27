import { useQuery } from "@tanstack/react-query";
import { MOCK_HOTELS, MOCK_ROOMS } from "lib/mockData";
import { type Hotel, type Room } from "shared/schema";

// NOTE: Using mock data instead of API calls as requested

export function useHotels() {
  return useQuery({
    queryKey: ['/api/hotels'],
    queryFn: async () => {
      // Simulate network delay
      await new Promise(resolve => setTimeout(resolve, 800));
      return MOCK_HOTELS;
    },
  });
}

export function useHotel(id: number) {
  return useQuery({
    queryKey: ['/api/hotels', id],
    queryFn: async () => {
      await new Promise(resolve => setTimeout(resolve, 600));
      const hotel = MOCK_HOTELS.find(h => h.id === id);
      if (!hotel) throw new Error("Hotel not found");
      return hotel;
    },
  });
}

export function useHotelRooms(hotelId: number) {
  return useQuery({
    queryKey: ['/api/hotels', hotelId, 'rooms'],
    queryFn: async () => {
      await new Promise(resolve => setTimeout(resolve, 700));
      // For mock purposes, just return some rooms if they match, or generic ones
      const rooms = MOCK_ROOMS.filter(r => r.hotelId === hotelId);
      // Fallback to generic rooms if none specifically assigned in mock
      return rooms.length > 0 ? rooms : MOCK_ROOMS.slice(0, 2); 
    },
  });
}
