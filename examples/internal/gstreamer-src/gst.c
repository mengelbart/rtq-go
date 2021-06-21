#include "gst.h"

GMainLoop *gstreamer_send_main_loop = NULL;
void gstreamer_send_start_mainloop(void) {
  gstreamer_send_main_loop = g_main_loop_new(NULL, FALSE);

  g_main_loop_run(gstreamer_send_main_loop);
}

static gboolean go_gst_bus_call(GstBus *bus, GstMessage *msg, gpointer data) {
    switch (GST_MESSAGE_TYPE(msg)) {

    case GST_MESSAGE_EOS: {
        goHandleEOS();
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

GstElement* gstreamer_send_create_pipeline(char *pipelineStr) {
    GError *error = NULL;
    GstElement *pipeline;

    gst_init(NULL, NULL);

    return gst_parse_launch(pipelineStr, &error);
}

void gstreamer_send_start_pipeline(GstElement* pipeline, int pipelineId) {
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

void gstreamer_send_stop_pipeline(GstElement* pipeline) {
    gst_element_send_event(pipeline, gst_event_new_eos());
}

void gstreamer_send_destroy_pipeline(GstElement* pipeline) {
    gst_element_set_state(pipeline, GST_STATE_NULL);
    gst_object_unref(pipeline);
}

unsigned int gstreamer_send_get_ssrc(GstElement* pipeline, char *payloader) {
    GstElement* rtph264pay = gst_bin_get_by_name(GST_BIN(pipeline), payloader);
    unsigned int ssrc = 0;
    g_object_get(rtph264pay, "ssrc", &ssrc, NULL);
    return ssrc;
}

void gstreamer_send_set_ssrc(GstElement* pipeline, char *payloader, unsigned int ssrc) {
    GstElement* rtph264pay = gst_bin_get_by_name(GST_BIN(pipeline), payloader);
    g_object_set(rtph264pay, "ssrc", ssrc, NULL);
}

void gstreamer_send_set_bitrate(GstElement* pipeline, unsigned int bitrate) {
    GstElement* x264enc = gst_bin_get_by_name(GST_BIN(pipeline), "x264enc");
    g_object_set(x264enc, "bitrate", bitrate, NULL);
}