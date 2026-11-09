import unittest

from tools.demo import DEMO_PASSENGER, DEMO_TICKET
from tools.excursions import search_trip_recommendations
from tools.flights import cancel_ticket, fetch_user_flight_information, search_flights, update_ticket_to_new_flight
from tools.hotels import book_hotel, cancel_hotel, search_hotels, update_hotel


def config(thread, passenger=DEMO_PASSENGER):
    return {"configurable": {"thread_id": thread, "passenger_id": passenger}}


class DemoBookingTests(unittest.TestCase):
    def test_ticket_changes_are_owned_and_isolated(self):
        first, second = config("flight-one"), config("flight-two")
        args = {"ticket_no": DEMO_TICKET, "new_flight_id": 2}
        self.assertIn("not found", update_ticket_to_new_flight.invoke(args, config("flight-one", "other")))
        self.assertEqual(len(search_flights.invoke({"departure_airport": "zrh"})), 2)
        self.assertIn("DEMO102", update_ticket_to_new_flight.invoke(args, first))
        self.assertEqual(fetch_user_flight_information.invoke({}, first)[0]["flight_id"], 2)
        self.assertEqual(fetch_user_flight_information.invoke({}, second)[0]["flight_id"], 1)
        self.assertIn("Simulated", cancel_ticket.invoke({"ticket_no": DEMO_TICKET}, first))
        self.assertEqual(fetch_user_flight_information.invoke({}, first), [])
        self.assertIn("already cancelled", update_ticket_to_new_flight.invoke(args, first))

    def test_hotel_dates_and_booking_are_session_local(self):
        first, second = config("hotel-one"), config("hotel-two")
        hotel_id = search_hotels.invoke({"location": "geneva"})[0]["id"]
        args = {"hotel_id": hotel_id}
        self.assertIn("Simulated", book_hotel.invoke(args, first))
        self.assertIn("No demo", cancel_hotel.invoke(args, second))
        self.assertIn("end date", update_hotel.invoke({
            **args, "checkin_date": "2030-01-10", "checkout_date": "2030-01-09"
        }, first))
        self.assertIn("Simulated", update_hotel.invoke({
            **args, "checkin_date": "2030-01-10", "checkout_date": "2030-01-11"
        }, first))
        self.assertIn("Simulated", cancel_hotel.invoke(args, first))

    def test_excursion_search_matches_any_keyword(self):
        for keywords in ("lake, sightseeing", "skiing LAKE", " , "):
            with self.subTest(keywords=keywords):
                results = search_trip_recommendations.invoke({"location": "Geneva", "keywords": keywords})
                self.assertEqual([item["name"] for item in results], ["Demo Lake Walk"])
        self.assertEqual(search_trip_recommendations.invoke({"keywords": "skiing, museum"}), [])
        self.assertEqual(search_trip_recommendations.invoke({"location": "Zurich", "keywords": "lake"}), [])
