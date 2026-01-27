import json
import re
import unicodedata
from pathlib import Path
from typing import Any, Dict, List

from reportlab.lib.pagesizes import letter
from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
from reportlab.lib.units import inch
from reportlab.platypus import Paragraph, SimpleDocTemplate, Spacer

REPO_ROOT = Path(__file__).resolve().parents[2]
OUTPUT_ROOT = REPO_ROOT / "resources" / "policy_pdfs"
DATASET_PATH = REPO_ROOT / "backend" / "agent" / "hotel" / "mock_dataset.json"
MAX_HOTELS = 5

EXTRA_POLICIES: Dict[str, List[str]] = {
    "Marino Beach Colombo": [
        "Credit card policy: A valid card is required to guarantee the reservation; charges may apply based on room type.",
        "Pet policy: Pets are not permitted.",
        "Pool policy: Rooftop pool hours are 6:00 AM to 8:00 PM; swim attire required. No glassware is permitted. Children under 12 must be supervised by an adult.",
    ],
    "Shangri-La Colombo": [
        "Deposit policy: A one-night deposit may be required for prepaid or promotional rates.",
        "Child policy: Children under 12 stay free when sharing existing bedding.",
        "Pool policy: Pool hours are 6:00 AM to 9:00 PM. Towels are provided at the pool deck. Children must be accompanied by an adult at all times.",
    ],
    "Cinnamon Grand Colombo": [
        "Check-in requirements: Photo ID and the card used for booking may be required at check-in.",
        "Smoking policy: Non-smoking rooms only; smoking fee may apply.",
        "Pool policy: Pool hours are 6:00 AM to 8:00 PM. No diving. Food and beverages from outside are not permitted.",
    ],
    "Hilton Colombo": [
        "Cancellation policy: Free cancellation up to 72 hours before arrival unless otherwise stated.",
        "Extra bed policy: Rollaway beds are available on request for an additional fee.",
        "Pool policy: Pool hours are 6:00 AM to 10:00 PM. Proper swimwear is required. Children must be supervised by an adult.",
    ],
    "Radisson Hotel Colombo": [
        "Group policy: Special terms may apply for bookings of 10 rooms or more.",
        "Early departure policy: Early departures may incur a fee equal to one night.",
        "Pool policy: Pool hours are 7:00 AM to 9:00 PM. No food or drink in the pool. Children must be supervised.",
    ],
}


def _slugify(value: str) -> str:
    normalized = unicodedata.normalize("NFKD", value)
    ascii_value = normalized.encode("ascii", "ignore").decode("ascii")
    slug = re.sub(r"[^a-zA-Z0-9]+", "_", ascii_value.lower()).strip("_")
    return slug or "hotel"


def _load_mock_hotels() -> List[Dict[str, Any]]:
    data = json.loads(DATASET_PATH.read_text())
    return data.get("hotels", [])[:MAX_HOTELS]


def _write_pdf_from_text(text: str, output_path: Path, hotel_name: str) -> None:
    styles = getSampleStyleSheet()
    styles.add(
        ParagraphStyle(
            name="PolicyText",
            parent=styles["Normal"],
            fontSize=10,
            leading=14,
            spaceAfter=12,
        )
    )
    doc = SimpleDocTemplate(
        str(output_path),
        pagesize=letter,
        rightMargin=72,
        leftMargin=72,
        topMargin=72,
        bottomMargin=18,
    )
    story: List[Any] = []
    story.append(Paragraph(f"{hotel_name} - Hotel Policies", styles["Heading1"]))
    story.append(Spacer(1, 0.2 * inch))
    for para in text.split("\n\n"):
        if para.strip():
            cleaned = para.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
            story.append(Paragraph(cleaned, styles["PolicyText"]))
            story.append(Spacer(1, 0.1 * inch))
    doc.build(story)


def _build_policy_text(hotel: Dict[str, Any]) -> str:
    policy = hotel.get("checkInOutPolicy") or {}
    parts = [
        f"Hotel: {hotel.get('hotelName', 'Unknown Hotel')}",
        f"Address: {hotel.get('address', '')}",
        f"City: {hotel.get('city', '')}",
        f"Check-in time: {policy.get('checkInTime', 'Not specified')}",
        f"Check-out time: {policy.get('checkOutTime', 'Not specified')}",
        f"Cancellation policy: {policy.get('cancellationPolicy', 'Not specified')}",
    ]
    amenities = hotel.get("amenities") or []
    if amenities:
        parts.append(f"Amenities: {', '.join(amenities)}")
    extras = EXTRA_POLICIES.get(hotel.get("hotelName") or "", [])
    if extras:
        parts.append("Additional policies:")
        parts.extend(f"- {item}" for item in extras)
    return "\n\n".join(parts)


def _process_hotel(hotel: Dict[str, Any]) -> None:
    hotel_name = hotel.get("hotelName") or "Unknown Hotel"
    slug = _slugify(hotel_name)
    hotel_folder = OUTPUT_ROOT / slug
    hotel_folder.mkdir(parents=True, exist_ok=True)

    pdf_path = hotel_folder / "policies.pdf"
    policy_text = _build_policy_text(hotel)
    _write_pdf_from_text(policy_text, pdf_path, hotel_name)

    metadata_path = hotel_folder / "metadata.json"
    metadata = {"hotelId": hotel.get("hotelId"), "hotelName": hotel_name}
    metadata_path.write_text(json.dumps(metadata, indent=2), encoding="utf-8")


def main() -> None:
    OUTPUT_ROOT.mkdir(parents=True, exist_ok=True)
    for hotel in _load_mock_hotels():
        _process_hotel(hotel)


if __name__ == "__main__":
    main()
