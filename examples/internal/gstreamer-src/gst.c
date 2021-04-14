#include "gst.h"

static gboolean go_gst_bus_call(GstBus *bus, GstMessage *msg, gpointer data) {
    switch (GST_MESSAGE_TYPE(msg)) {

    case GST_MESSAGE_EOS: {
        g_print("Unexpected end of stream signal\n");
        exit(1);
        break;
    }

    case GST_MESSAGE_ERROR: {
        gchar *debug;
        GError *error;

        gst_message_parse_error(msg, &error, &debug);
        g_free(debug);

        g_printerr("Error: %s\n", error->message);
        g_error_free(error);
        exit(1);
    }

    default:
        break;
    }

    return TRUE;
}

GstFlowReturn go_gst_send_new_sample_handler(GstElement *object, gpointer user_data) {
    GstSample *sample = NULL;
    GstBuffer *buffer = NULL;
    gpointer copy = NULL;
    gsize copy_size = 0;
    SampleHandlerUserData *s = (SampleHandlerUserData*) user_data;

    g_signal_emit_by_name (object, "pull-sample", &sample);

    if (sample) {
        buffer = gst_sample_get_buffer(sample);
        if (buffer) {
            gst_buffer_extract_dup(buffer, 0, gst_buffer_get_size(buffer), &copy, &copy_size);
            goHandlePipelineBuffer(copy, copy_size, s->pipelineId);
        }
        gst_sample_unref(sample);
    }

    return GST_FLOW_OK;
}

GstElement* go_gst_create_src_pipeline(char *pipelineStr) {
    GError *error = NULL;
    GstElement *pipeline;

    gst_init(NULL, NULL);

    return gst_parse_launch(pipelineStr, &error);
}

void go_gst_start_src_pipeline(GstElement* pipeline, int pipelineId) {
    SampleHandlerUserData* s = malloc(sizeof(SampleHandlerUserData));
    s->pipelineId = pipelineId;

    GstBus *bus = gst_pipeline_get_bus(GST_PIPELINE(pipeline));
    gst_bus_add_watch(bus, go_gst_bus_call, NULL);
    gst_object_unref(bus);

    GstElement *appsink = gst_bin_get_by_name(GST_BIN(pipeline), "appsink");
    g_object_set(appsink, "emit-signals", TRUE, NULL);
    g_signal_connect(appsink, "new-sample", G_CALLBACK(go_gst_send_new_sample_handler), s);
    gst_object_unref(appsink);

    gst_element_set_state(pipeline, GST_STATE_PLAYING);
}

unsigned int go_gst_get_ssrc(GstElement* pipeline) {
    GstElement* rtph264pay = gst_bin_get_by_name(GST_BIN(pipeline), "rtph264pay");
    unsigned int ssrc = 0;
    g_object_get(rtph264pay, "ssrc", &ssrc, NULL);
    return ssrc;
}

void go_gst_set_ssrc(GstElement* pipeline, unsigned int ssrc) {
    GstElement* rtph264pay = gst_bin_get_by_name(GST_BIN(pipeline), "rtph264pay");
    g_object_set(rtph264pay, "ssrc", ssrc, NULL);
}

void go_gst_set_bitrate(GstElement* pipeline, unsigned int bitrate) {
    GstElement* x264enc = gst_bin_get_by_name(GST_BIN(pipeline), "x264enc");
    g_object_set(x264enc, "bitrate", bitrate, NULL);
}