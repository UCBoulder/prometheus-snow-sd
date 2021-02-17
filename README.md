# Servicenow service discover for Prometheus

## TODO

This currently chokes on parsing the JSON response from SN, and apparently juggling arbitrary data
objects is harder in static-typed languages like Go than the free-for-all we're used to with python
and ansible.

## Building

To build this code, simply run the following from the root directory of this repo:

    go build

## Usage

    export SNOW_USER=wiha1292
    read -s SNOW_PASS
    ....
    export SNOW_PASS
    ./prometheus-snow-sd --snow.instance=coloradodev --sysparm.query="u_systemlist_customer_organization=sepe"