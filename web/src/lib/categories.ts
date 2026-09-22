import type { Category } from './types';

export type CategoryInfo = { id: Category; label: string; emoji: string };

/** Ordered list used for pickers and charts. */
export const CATEGORIES: CategoryInfo[] = [
  { id: 'food', label: 'Food', emoji: '🍽️' },
  { id: 'groceries', label: 'Groceries', emoji: '🛒' },
  { id: 'drinks', label: 'Drinks', emoji: '🍻' },
  { id: 'transport', label: 'Transport', emoji: '🚕' },
  { id: 'accommodation', label: 'Stay', emoji: '🏠' },
  { id: 'entertainment', label: 'Fun', emoji: '🎟️' },
  { id: 'shopping', label: 'Shopping', emoji: '🛍️' },
  { id: 'utilities', label: 'Utilities', emoji: '💡' },
  { id: 'health', label: 'Health', emoji: '💊' },
  { id: 'travel', label: 'Travel', emoji: '✈️' },
  { id: 'gifts', label: 'Gifts', emoji: '🎁' },
  { id: 'other', label: 'Other', emoji: '📎' },
];

const BY_ID: Record<string, CategoryInfo> = Object.fromEntries(CATEGORIES.map((c) => [c.id, c]));
const OTHER = BY_ID.other;

/** Normalise anything (including undefined from older data) to a known category. */
export function normalizeCategory(c: unknown): Category {
  return typeof c === 'string' && c in BY_ID ? (c as Category) : 'other';
}

export function categoryInfo(c: unknown): CategoryInfo {
  return BY_ID[normalizeCategory(c)] ?? OTHER;
}

// Keyword -> category. Longer / more specific keywords first so "ice cream" beats "cream" etc.
const KEYWORDS: [string, Category][] = [
  // transport
  ...['uber', 'ola', 'lyft', 'grab', 'bolt', 'taxi', 'cab', 'metro', 'subway', 'tram', 'bus', 'fuel', 'petrol', 'diesel', 'gas station', 'parking', 'toll', 'rickshaw', 'auto', 'scooter', 'bike rental', 'car rental', 'rental car'].map((k): [string, Category] => [k, 'transport']),
  // travel
  ...['flight', 'airline', 'airfare', 'indigo', 'emirates', 'ryanair', 'easyjet', 'train ticket', 'railway', 'irctc', 'eurail', 'ferry', 'cruise', 'visa', 'passport', 'insurance', 'boarding', 'luggage', 'baggage', 'airport'].map((k): [string, Category] => [k, 'travel']),
  // accommodation
  ...['hotel', 'airbnb', 'hostel', 'motel', 'resort', 'villa', 'apartment', 'rent', 'lodge', 'guesthouse', 'homestay', 'booking.com', 'oyo', 'deposit'].map((k): [string, Category] => [k, 'accommodation']),
  // groceries
  ...['grocery', 'groceries', 'supermarket', 'market', 'veg', 'vegetables', 'fruit', 'milk', 'eggs', 'bread', 'rice', 'costco', 'walmart', 'tesco', 'lidl', 'aldi', 'carrefour', 'bigbasket', 'blinkit', 'zepto', 'instamart', 'lulu', 'spinneys'].map((k): [string, Category] => [k, 'groceries']),
  // drinks
  ...['beer', 'wine', 'whisky', 'whiskey', 'vodka', 'gin', 'rum', 'cocktail', 'bar', 'pub', 'brewery', 'drinks', 'booze', 'liquor', 'shots', 'champagne', 'prosecco'].map((k): [string, Category] => [k, 'drinks']),
  // food
  ...['pizza', 'burger', 'dinner', 'lunch', 'breakfast', 'brunch', 'restaurant', 'cafe', 'café', 'coffee', 'starbucks', 'tea', 'snack', 'snacks', 'food', 'meal', 'dominos', "domino's", 'kfc', 'mcdonald', 'subway sandwich', 'sushi', 'biryani', 'shawarma', 'kebab', 'thali', 'dosa', 'zomato', 'swiggy', 'deliveroo', 'talabat', 'doordash', 'ubereats', 'takeaway', 'takeout', 'ice cream', 'dessert', 'bakery', 'chai'].map((k): [string, Category] => [k, 'food']),
  // entertainment
  ...['movie', 'cinema', 'film', 'concert', 'gig', 'show', 'theatre', 'theater', 'museum', 'zoo', 'park ticket', 'theme park', 'game', 'games', 'bowling', 'karaoke', 'club', 'netflix', 'spotify', 'disney', 'prime video', 'playstation', 'steam', 'match', 'ipl', 'stadium', 'safari', 'tour', 'scuba', 'surf', 'kayak', 'trek'].map((k): [string, Category] => [k, 'entertainment']),
  // utilities
  ...['electricity', 'electric bill', 'power bill', 'water bill', 'gas bill', 'wifi', 'internet', 'broadband', 'phone bill', 'mobile bill', 'recharge', 'sim card', 'du ', 'etisalat', 'jio', 'airtel', 'utility', 'utilities', 'maintenance', 'cleaning', 'laundry', 'dewa'].map((k): [string, Category] => [k, 'utilities']),
  // health
  ...['pharmacy', 'chemist', 'doctor', 'clinic', 'hospital', 'medicine', 'medicines', 'meds', 'dentist', 'gym', 'yoga', 'physio', 'vitamins', 'first aid', 'sunscreen'].map((k): [string, Category] => [k, 'health']),
  // gifts
  ...['gift', 'present', 'birthday', 'anniversary', 'wedding', 'flowers', 'bouquet', 'souvenir', 'souvenirs'].map((k): [string, Category] => [k, 'gifts']),
  // shopping
  ...['amazon', 'noon', 'flipkart', 'mall', 'clothes', 'clothing', 'shoes', 'sneakers', 'shirt', 'dress', 'jeans', 'ikea', 'decathlon', 'zara', 'h&m', 'uniqlo', 'electronics', 'charger', 'cable', 'headphones', 'shopping', 'duty free', 'stationery'].map((k): [string, Category] => [k, 'shopping']),
];

/**
 * Suggest a category from free text. Substring match on the lowercased description,
 * longest keyword wins so more specific phrases beat generic ones.
 */
export function suggestCategory(description: string): Category | null {
  const text = ` ${description.toLowerCase().trim()} `;
  if (text.trim() === '') return null;
  let best: { cat: Category; len: number } | null = null;
  for (const [kw, cat] of KEYWORDS) {
    if (!text.includes(kw)) continue;
    // Short generic words must match as whole words (plural allowed) to avoid "bar" in "barbecue".
    if (kw.length <= 4 && !new RegExp(`(^|[^a-z])${kw.trim().replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}(s|es)?([^a-z]|$)`).test(text)) continue;
    if (!best || kw.length > best.len) best = { cat, len: kw.length };
  }
  return best?.cat ?? null;
}
