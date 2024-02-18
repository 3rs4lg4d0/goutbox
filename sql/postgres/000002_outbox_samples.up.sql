-- Install the uuid-ossp extension if not already installed
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Insert statements with uuid_generate_v4()
INSERT INTO outbox (id, aggregate_type, aggregate_id, event_type, payload, created_at)
VALUES
  (uuid_generate_v4(), 'type1', 'id1', 'RestaurantCreated', E'\\x7B226B657931223A202256616C756531222C20226B657932223A202256616C756532227D', NOW()),
  (uuid_generate_v4(), 'type2', 'id2', 'RestaurantCreated', E'\\x7B226B657933223A202256616C756533222C20226B657934223A202256616C756534227D', NOW()),
  (uuid_generate_v4(), 'type3', 'id3', 'RestaurantCreated', E'\\x7B226B657935223A202256616C756535222C20226B657936223A202256616C756536227D', NOW()),
  (uuid_generate_v4(), 'type10', 'id10', 'RestaurantCreated', E'\\x7B226B65793130223A202256616C75653130222C20226B65793131223A202256616C75653131227D', NOW());
