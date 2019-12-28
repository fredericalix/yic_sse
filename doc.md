# SSE update

SSE or [Server-Sent Events](https://www.w3.org/TR/eventsource/)

    GET https://youritcity.io/sse
    Header : Authorization: Bearer sdfd54fdsSFDdsfd84fsdSD8
    or Cookie: session_token=sdfd54fdsSFDdsfd84fsdSD8

There is 3 types of event:

* layout
* sensors

## ui.layout event

The event name is `ui.layout`.
The event's data contains an update city layout in json format, the same as the POST one.

## sensors event

The event name is `sensors`.
The event data is json formated like the following:

``` json
{
    "id": "339e0d4e-39fe-4c03-9f85-f49e3fa97e31",
    "enable": true,
    "activity": "normal",
    "status": "online",
    "ram": 2,
    "ramusage": 1,
    "loadaverage": 1.23
}
```
