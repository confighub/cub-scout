#!/bin/bash

# Be careful running this if you have anything else in your organization

cub space delete --recursive --where "Slug LIKE 'app%'"
cub space delete --recursive --where "Slug LIKE 'platform%'"
cub space delete --recursive home
kind delete cluster --name dev
kind delete cluster --name prod
