import requests
import uuid
import time
import sys

BASE_URL = "http://localhost:8080/api/v1"

def test_api():
    print("Starting E2E API tests...")
    
    # Generate unique test data
    unique_id = str(uuid.uuid4())[:8]
    username = f"testuser_{unique_id}"
    email = f"test_{unique_id}@example.com"
    password = "StrongPassword123!"
    
    # 1. Register
    print(f"Testing Registration: {username}")
    reg_payload = {
        "username": username,
        "email": email,
        "password": password
    }
    resp = requests.post(f"{BASE_URL}/auth/register", json=reg_payload)
    if resp.status_code != 201:
        print(f"FAIL: Registration failed with {resp.status_code}")
        print(resp.text)
        sys.exit(1)
    
    auth_data = resp.json()["data"]
    access_token = auth_data["accessToken"]
    print("SUCCESS: Registration successful")

    headers = {
        "Authorization": f"Bearer {access_token}"
    }

    # 2. Login
    print("Testing Login")
    login_payload = {
        "login": username,
        "password": password
    }
    resp = requests.post(f"{BASE_URL}/auth/login", json=login_payload)
    assert resp.status_code == 200, f"Login failed: {resp.text}"
    print("SUCCESS: Login successful")

    # 3. Get Me
    print("Testing Get Current User")
    resp = requests.get(f"{BASE_URL}/users/me", headers=headers)
    assert resp.status_code == 200, f"Get current user failed: {resp.text}"
    user_data = resp.json()["data"]
    assert user_data["username"] == username
    print(f"SUCCESS: Retrieved user {user_data['username']}")

    # 4. Create Community
    print("Testing Create Community")
    comm_payload = {
        "name": f"Test Community {unique_id}",
        "description": "A community created by E2E tests"
    }
    resp = requests.post(f"{BASE_URL}/communities", json=comm_payload, headers=headers)
    assert resp.status_code == 201, f"Create community failed: {resp.text}"
    community_data = resp.json()["data"]
    community_id = community_data["id"]
    print(f"SUCCESS: Created community {community_id}")

    # 5. Create Channel
    print("Testing Create Channel")
    chan_payload = {
        "name": "general",
        "type": "text",
        "topic": "General discussion"
    }
    # Backend API: /api/v1/channels/communities/{id}/channels
    resp = requests.post(f"{BASE_URL}/channels/communities/{community_id}/channels", json=chan_payload, headers=headers)
    assert resp.status_code == 201, f"Create channel failed: {resp.text}"
    channel_data = resp.json()["data"]
    channel_id = channel_data["id"]
    print(f"SUCCESS: Created channel {channel_id}")

    # 6. Send Message
    print("Testing Send Message")
    msg_payload = {
        "content": "Hello from E2E test!"
    }
    # Backend API: /api/v1/messages/channels/{id}/messages
    resp = requests.post(f"{BASE_URL}/messages/channels/{channel_id}/messages", json=msg_payload, headers=headers)
    assert resp.status_code == 201, f"Send message failed: {resp.text}"
    message_data = resp.json()["data"]
    print(f"SUCCESS: Sent message {message_data['id']}")

    # 7. Get Messages
    print("Testing Get Messages")
    # Backend API: /api/v1/messages/channels/{id}/messages
    resp = requests.get(f"{BASE_URL}/messages/channels/{channel_id}/messages", headers=headers)
    assert resp.status_code == 200, f"Get messages failed: {resp.text}"
    messages = resp.json()["data"]
    assert len(messages) >= 1
    message_id = messages[0]["id"] 
    print(f"SUCCESS: Retrieved {len(messages)} messages")

    # 8. Add Reaction
    print("Testing Add Reaction")
    react_payload = {"emoji": "😭"} # Note for future me, please change reactions to send as ID's, not unicode charectors.
    resp = requests.post(f"{BASE_URL}/messages/{message_id}/reactions", json=react_payload, headers=headers)
    assert resp.status_code == 204, f"Add reaction failed: {resp.text}"
    print("SUCCESS: Added reaction")

    # 9. Get Message with Reaction
    print("Testing Get Message with Reaction")
    resp = requests.get(f"{BASE_URL}/messages/{message_id}", headers=headers)
    assert resp.status_code == 200, f"Get message failed: {resp.text}"
    msg_with_react = resp.json()["data"]
    assert len(msg_with_react["reactions"]) == 1
    assert msg_with_react["reactions"][0]["emoji"] == "😭"
    print("SUCCESS: Verified reaction in message data")

    # 10. Update Profile
    print("Testing Update Profile")
    update_payload = {
        "bio": "I am a bot for E2E testing"
    }
    resp = requests.patch(f"{BASE_URL}/users/me", json=update_payload, headers=headers)
    assert resp.status_code == 200, f"Update profile failed: {resp.text}"
    print("SUCCESS: Updated profile bio")

    # 9. Delete Message
    print("Testing Delete Message")
    resp = requests.delete(f"{BASE_URL}/messages/{message_id}", headers=headers)
    assert resp.status_code == 204, f"Delete message failed: {resp.text}"
    print(f"SUCCESS: Deleted message {message_id}")

    # 10. List Communities
    print("Testing List Communities")
    resp = requests.get(f"{BASE_URL}/communities", headers=headers)
    assert resp.status_code == 200, f"List communities failed: {resp.text}"
    communities = resp.json()["data"]
    assert len(communities) >= 1
    print(f"SUCCESS: Found {len(communities)} communities")

    print("\nAll E2E API tests passed successfully!")

if __name__ == "__main__":
    try:
        test_api()
    except Exception as e:
        print(f"\nERROR: Tests failed with unexpected error: {e}")
        sys.exit(1)
