export type Hotel = {
  id: number;
  name: string;
  location: string;
  description: string;
  rating: string | number;
  reviewCount: number;
  pricePerNight: number;
  imageUrl: string;
  website?: string;
  email?: string;
  latitude?: number;
  longitude?: number;
  amenities: string[];
  tags: string[];
};

export type Room = {
  id: number;
  hotelId: number;
  name: string;
  description: string;
  capacity: number;
  price: number;
  amenities: string[];
};

export type Booking = {
  id: number;
  hotelId: number;
  guestName: string;
  checkIn: string;
  checkOut: string;
  guests: number;
  totalPrice: number;
};
