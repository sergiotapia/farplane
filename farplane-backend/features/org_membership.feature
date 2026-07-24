Feature: Organization membership
  Critical path: owner membership after setup and member listing.

  Scenario: Setup owner appears in organization members
    Given a clean Farplane API
    When I set up an organization "Acme Co" as "owner@example.com" with password "password1"
    Then the response status is 201
    And I have a session cookie
    When I get "/api/v1/organization-members"
    Then the response status is 200
    And the organization members include "owner@example.com"
    When I get "/api/v1/me"
    Then the response status is 200
    And the JSON field "organization.role" is "owner"

  Scenario: Second setup is rejected after the organization exists
    Given a clean Farplane API
    And an organization "Acme Co" owned by "owner@example.com" with password "password1"
    When I set up an organization "Acme Co" as "other@example.com" with password "password1"
    Then the response status is 409
